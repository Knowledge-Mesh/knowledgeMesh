package mesh

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/matchmaker"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

// Runtime wires buyer session, matchmaking, and libp2p inference calls.
type Runtime struct {
	Buyer   *buyer.Manager
	Match   *matchmaker.Service
	Host    host.Host
	mu      sync.RWMutex
	sellers []types.SellerNode
}

func NewRuntime(b *buyer.Manager, h host.Host) *Runtime {
	return &Runtime{
		Buyer: b,
		Match: matchmaker.NewService(),
		Host:  h,
	}
}

func (r *Runtime) SetSellers(nodes []types.SellerNode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sellers = append([]types.SellerNode(nil), nodes...)
}

func (r *Runtime) Sellers() []types.SellerNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]types.SellerNode(nil), r.sellers...)
}

func (r *Runtime) Register(email, username, password string) (buyer.State, error) {
	return r.Buyer.Register(email, username, password)
}

func (r *Runtime) Login(userOrEmail, password string) (buyer.State, error) {
	return r.Buyer.Login(userOrEmail, password)
}

func (r *Runtime) RunInference(ctx context.Context, sessionID string, req types.InferenceRequest) (types.InferenceResponse, error) {
	if strings.TrimSpace(sessionID) == "" {
		return types.InferenceResponse{}, buyer.ErrInvalidSession
	}

	est := estimateTokens(req.Input)
	now := time.Now().UTC()
	if err := r.Buyer.CheckLimits(sessionID, est, now); err != nil {
		return types.InferenceResponse{}, err
	}

	st, err := r.Buyer.GetState(sessionID)
	if err != nil {
		return types.InferenceResponse{}, err
	}
	if req.Skill.Name == "" {
		if st.Preferences.PreferredSkill != "" {
			req.Skill.Name = st.Preferences.PreferredSkill
		} else {
			req.Skill.Name = "chat"
		}
	}
	if st.Preferences.MaxPricePerRequest > 0 && req.MaxPrice == 0 {
		req.MaxPrice = st.Preferences.MaxPricePerRequest
	}
	req.BuyerPeerID = st.BuyerID
	if req.RequestID == "" {
		req.RequestID = "req-" + sessionID + "-" + time.Now().Format("150405.000000000")
	}

	sel, err := r.Match.Match(req, r.Sellers())
	if err != nil {
		return types.InferenceResponse{}, err
	}

	pid, err := peer.Decode(sel.PeerID)
	if err != nil {
		return types.InferenceResponse{}, err
	}

	body, err := json.Marshal(req)
	if err != nil {
		return types.InferenceResponse{}, err
	}

	respBytes, err := network.SendRequest(ctx, r.Host, pid, network.ProtocolInference, body)
	if err != nil {
		return types.InferenceResponse{}, err
	}

	var resp types.InferenceResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return types.InferenceResponse{}, err
	}

	if resp.Success {
		tok := resp.TokenUsage.TotalTokens
		if tok <= 0 {
			tok = est
		}
		_, _ = r.Buyer.ConsumeUsage(sessionID, req.RequestID, req.Input, tok, sel.Price, now)
	}
	return resp, nil
}

func estimateTokens(v string) int {
	if strings.TrimSpace(v) == "" {
		return 0
	}
	return len(strings.Fields(v))
}
