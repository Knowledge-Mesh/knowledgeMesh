package mesh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/buyer"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

// Runtime wires buyer session, control-plane matchmaking, and libp2p inference calls.
type Runtime struct {
	Buyer   *buyer.Manager
	Control *control.Client
	Host    host.Host
}

func NewRuntime(b *buyer.Manager, h host.Host) *Runtime {
	return &Runtime{
		Buyer: b,
		Host:  h,
	}
}

// Register creates a buyer account on the control pane (no local-only registration).
func (r *Runtime) Register(name, email, password string) (buyer.State, error) {
	if r.Control == nil {
		return buyer.State{}, errors.New("control pane not configured; use --control-url")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return buyer.State{}, errors.New("name is required")
	}
	id, err := r.Control.RegisterBuyer(name, email, password)
	if err != nil {
		return buyer.State{}, err
	}
	email = strings.TrimSpace(strings.ToLower(email))
	return buyer.State{
		BuyerID: id,
		AuthRef: "control:" + email,
	}, nil
}

// Login authenticates against the control pane and establishes a local session (JWT is the session token).
func (r *Runtime) Login(userOrEmail, password string) (buyer.State, error) {
	if r.Control == nil {
		return buyer.State{}, errors.New("control pane not configured; use --control-url")
	}
	tok, buyerID, name, email, err := r.Control.LoginBuyer(userOrEmail, password)
	if err != nil {
		return buyer.State{}, err
	}
	return r.Buyer.EstablishControlSession(name, email, buyerID, tok)
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

	if r.Control == nil {
		return types.InferenceResponse{}, errors.New("control pane client is required for inference (matchmaking and billing)")
	}

	match, err := r.Control.PostInferenceMatch(sessionID, req)
	if err != nil {
		return types.InferenceResponse{}, err
	}
	req.RequestID = match.RequestID
	req.BuyerPeerID = r.Host.ID().String()

	trackMeta := map[string]any{
		"buyerP2pPeerId": r.Host.ID().String(),
		"modelName":      req.ModelName,
		"skill":          req.Skill.Name,
	}
	_ = r.Control.PostBuyerInferenceTracking(sessionID, req.RequestID, "started", trackMeta)

	pid, err := peer.Decode(match.SellerPeerID)
	if err != nil {
		return types.InferenceResponse{}, fmt.Errorf("seller peer id: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return types.InferenceResponse{}, err
	}

	respBytes, err := network.SendRequest(ctx, r.Host, pid, network.ProtocolInference, body)
	if err != nil {
		_ = r.Control.PostBuyerInferenceComplete(sessionID, req.RequestID, 0, false, map[string]any{"error": err.Error()})
		return types.InferenceResponse{}, err
	}

	var resp types.InferenceResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		_ = r.Control.PostBuyerInferenceComplete(sessionID, req.RequestID, 0, false, map[string]any{"error": err.Error()})
		return types.InferenceResponse{}, err
	}

	tok := int64(resp.TokenUsage.TotalTokens)
	if resp.Success && tok <= 0 {
		tok = int64(est)
	}
	_ = r.Control.PostBuyerInferenceTracking(sessionID, req.RequestID, "completed", map[string]any{
		"success":     resp.Success,
		"totalTokens": tok,
		"sellerId":    match.SellerID,
	})
	_ = r.Control.PostBuyerInferenceComplete(sessionID, req.RequestID, tok, resp.Success, map[string]any{
		"sellerPeerId": match.SellerPeerID,
	})

	if resp.Success {
		ptok := resp.TokenUsage.TotalTokens
		if ptok <= 0 {
			ptok = est
		}
		_, _ = r.Buyer.ConsumeUsage(sessionID, req.RequestID, req.Input, ptok, match.Price, now)
	}
	return resp, nil
}

func estimateTokens(v string) int {
	if strings.TrimSpace(v) == "" {
		return 0
	}
	return len(strings.Fields(v))
}
