package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/knowledgemeshgrid/knowledgemesh/internal/matchmaker"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

func (s *HTTPServer) matcher() *matchmaker.Service {
	if s.Matcher != nil {
		return s.Matcher
	}
	return matchmaker.NewService()
}

func (s *HTTPServer) handleBuyerInferenceMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := bearerToken(r)
	buyerID, _, _, err := ParseBuyerToken(s.Secret, tok)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req types.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.RequestID) == "" {
		req.RequestID = "req-" + buyerID + "-" + uuid.New().String()
	}
	est := int64(estimateTokensForMatch(req.Input))
	if err := s.Store.CheckBuyerCanSpendTokens(buyerID, est); err != nil {
		if errors.Is(err, ErrInsufficientBalance) || errors.Is(err, ErrInferenceQuotaExceeded) {
			writeJSONErr(w, http.StatusPaymentRequired, err.Error())
			return
		}
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	nodes, err := s.Store.ListSellerNodesForMatch()
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sel, err := s.matcher().Match(req, nodes)
	if err != nil {
		if errors.Is(err, matchmaker.ErrNoSellerMatch) {
			writeJSONErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sellerID, err := s.Store.FindSellerIDByPeerID(sel.PeerID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "could not resolve seller for peer")
		return
	}
	if err := s.Store.InsertInferenceMatch(req.RequestID, buyerID, sellerID, sel.PeerID, int(est)); err != nil {
		if errors.Is(err, ErrDuplicateInferenceMatch) {
			writeJSONErr(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"requestId":          req.RequestID,
		"sellerId":           sellerID,
		"sellerPeerId":       sel.PeerID,
		"sellerListenAddrs":  sel.ListenAddrs,
		"price":              sel.Price,
		"reputation":         sel.Reputation,
	})
}

type inferenceTrackingReq struct {
	RequestID string         `json:"requestId"`
	Phase     string         `json:"phase"`
	Meta      map[string]any `json:"meta"`
}

func (s *HTTPServer) handleBuyerInferenceTracking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := bearerToken(r)
	buyerID, _, _, err := ParseBuyerToken(s.Secret, tok)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body inferenceTrackingReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.RequestID) == "" {
		writeJSONErr(w, http.StatusBadRequest, "requestId is required")
		return
	}
	phase := strings.TrimSpace(strings.ToLower(body.Phase))
	if phase == "" {
		phase = "event"
	}
	if err := s.Store.AppendBuyerInferenceTracking(buyerID, body.RequestID, phase, body.Meta); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type inferenceCompleteReq struct {
	RequestID   string         `json:"requestId"`
	TotalTokens int64          `json:"totalTokens"`
	Success     bool           `json:"success"`
	Meta        map[string]any `json:"meta"`
}

func (s *HTTPServer) handleBuyerInferenceComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := bearerToken(r)
	buyerID, _, _, err := ParseBuyerToken(s.Secret, tok)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body inferenceCompleteReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.RequestID) == "" {
		writeJSONErr(w, http.StatusBadRequest, "requestId is required")
		return
	}
	if err := s.Store.SettleInferenceMatch(buyerID, body.RequestID, body.TotalTokens, body.Success); err != nil {
		if errors.Is(err, ErrInferenceMatchNotFound) {
			writeJSONErr(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, ErrInsufficientBalance) || errors.Is(err, ErrInferenceQuotaExceeded) {
			writeJSONErr(w, http.StatusPaymentRequired, err.Error())
			return
		}
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type sellerInferenceTrackingReq struct {
	RequestID   string         `json:"requestId"`
	TotalTokens int64          `json:"totalTokens"`
	Success     bool           `json:"success"`
	Meta        map[string]any `json:"meta"`
}

func (s *HTTPServer) handleSellerInferenceTracking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := bearerToken(r)
	sellerID, _, _, err := ParseSellerToken(s.Secret, tok)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body sellerInferenceTrackingReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.RequestID) == "" {
		writeJSONErr(w, http.StatusBadRequest, "requestId is required")
		return
	}
	matchSeller, err := s.Store.GetInferenceMatchSeller(body.RequestID)
	if err != nil {
		writeJSONErr(w, http.StatusNotFound, err.Error())
		return
	}
	if matchSeller != sellerID {
		writeJSONErr(w, http.StatusForbidden, "request does not belong to this seller")
		return
	}
	if err := s.Store.AppendSellerInferenceTracking(sellerID, body.RequestID, body.TotalTokens, body.Success, body.Meta); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
