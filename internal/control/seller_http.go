package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

type sellerRegisterReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sellerDutyReq struct {
	OnDuty bool `json:"onDuty"`
}

type sellerModelsPutReq struct {
	Models []SellerModelRecord `json:"models"`
}

type sellerPresenceReq struct {
	PeerID      string   `json:"peerId"`
	ListenAddrs []string `json:"listenAddrs"`
}

func sellerProfileJSON(prof SellerProfile) map[string]any {
	m := map[string]any{
		"sellerId":    prof.SellerID,
		"name":        prof.Name,
		"email":       prof.Email,
		"onDuty":      prof.OnDuty,
		"peerId":      prof.PeerID,
		"listenAddrs": prof.ListenAddrs,
		"models":      prof.Models,
	}
	if prof.Ollama != nil {
		m["ollama"] = prof.Ollama
	}
	return m
}

func (s *HTTPServer) handleSellerRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body sellerRegisterReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	id, err := s.Store.RegisterSeller(body.Name, body.Email, body.Password)
	if err != nil {
		if errors.Is(err, ErrSellerExists) {
			writeJSONErr(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"sellerId": id,
		"email":    strings.TrimSpace(strings.ToLower(body.Email)),
		"name":     strings.TrimSpace(body.Name),
	})
}

func (s *HTTPServer) handleSellerLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sellerID, name, email, err := s.Store.LoginSeller(body.Email, body.Password)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := IssueSellerToken(s.Secret, sellerID, email, name)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	prof, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	m := sellerProfileJSON(prof)
	m["accessToken"] = token
	m["sellerId"] = sellerID
	m["name"] = name
	m["email"] = email
	_ = json.NewEncoder(w).Encode(m)
}

func (s *HTTPServer) sellerIDFromRequest(r *http.Request) (string, error) {
	tok := bearerToken(r)
	if tok == "" {
		return "", errors.New("missing bearer token")
	}
	sid, _, _, err := ParseSellerToken(s.Secret, tok)
	return sid, err
}

func (s *HTTPServer) handleSellerMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sellerID, err := s.sellerIDFromRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	prof, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sellerProfileJSON(prof))
}

func (s *HTTPServer) handleSellerDuty(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sellerID, err := s.sellerIDFromRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var body sellerDutyReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.Store.SetSellerDuty(sellerID, body.OnDuty); err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	prof, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sellerProfileJSON(prof))
}

func (s *HTTPServer) handleSellerModelsPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sellerID, err := s.sellerIDFromRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var body sellerModelsPutReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.Store.ReplaceSellerModels(sellerID, body.Models); err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	prof, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sellerProfileJSON(prof))
}

func (s *HTTPServer) handleSellerModelPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sellerID, err := s.sellerIDFromRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	modelID := strings.TrimSpace(r.PathValue("id"))
	if modelID == "" {
		writeJSONErr(w, http.StatusBadRequest, "missing model id")
		return
	}
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	prof, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	var found *SellerModelRecord
	for i := range prof.Models {
		if prof.Models[i].ID == modelID {
			found = &prof.Models[i]
			break
		}
	}
	if found == nil {
		writeJSONErr(w, http.StatusNotFound, "model not found")
		return
	}
	if v, ok := patch["active"].(bool); ok {
		found.Active = v
	}
	// merge other fields optionally
	if v, ok := patch["name"].(string); ok {
		found.Name = v
	}
	if v, ok := patch["version"].(string); ok {
		found.Version = v
	}
	if v, ok := patch["skillName"].(string); ok {
		found.SkillName = v
	}
	if v, ok := patch["modelName"].(string); ok {
		found.ModelName = v
	}
	if v, ok := patch["modelType"].(string); ok {
		found.ModelType = v
	}
	if v, ok := patch["tuningTier"].(string); ok {
		found.TuningTier = v
	}
	if v, ok := patch["hourlyTokens"].(float64); ok {
		found.HourlyTokens = int(v)
	}
	if v, ok := patch["dailyTokens"].(float64); ok {
		found.DailyTokens = int(v)
	}
	if v, ok := patch["totalTokens"].(float64); ok {
		found.TotalTokens = int(v)
	}
	if v, ok := patch["ratePerToken"].(float64); ok {
		found.RatePerToken = v
	}
	if err := s.Store.ReplaceSellerModels(sellerID, prof.Models); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	prof2, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sellerProfileJSON(prof2))
}

func (s *HTTPServer) handleSellerPresence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sellerID, err := s.sellerIDFromRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var body sellerPresenceReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.Store.SetSellerPresence(sellerID, body.PeerID, body.ListenAddrs); err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	prof, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sellerProfileJSON(prof))
}

func (s *HTTPServer) handleSellerOllama(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sellerID, err := s.sellerIDFromRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var cfg *types.OllamaSellerConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.Store.SetSellerOllamaConfig(sellerID, cfg); err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	prof, err := s.Store.GetSellerProfile(sellerID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sellerProfileJSON(prof))
}
