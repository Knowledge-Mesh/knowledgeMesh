package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/matchmaker"
)

// HTTPServer exposes buyer and seller registration/login and seller profile APIs (PostgresStore).
type HTTPServer struct {
	Store   *PostgresStore
	Secret  []byte
	Matcher *matchmaker.Service
}

type registerReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Handler serves control pane HTTP routes (buyers + sellers).
func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "module": "control"})
	})
	mux.HandleFunc("/v1/control/buyers/register", s.handleBuyerRegister)
	mux.HandleFunc("/v1/control/buyers/login", s.handleBuyerLogin)
	mux.HandleFunc("/v1/control/sellers/register", s.handleSellerRegister)
	mux.HandleFunc("/v1/control/sellers/login", s.handleSellerLogin)
	mux.HandleFunc("GET /v1/control/sellers/me", s.handleSellerMe)
	mux.HandleFunc("PUT /v1/control/sellers/me/duty", s.handleSellerDuty)
	mux.HandleFunc("PUT /v1/control/sellers/me/models", s.handleSellerModelsPut)
	mux.HandleFunc("PATCH /v1/control/sellers/me/models/{id}", s.handleSellerModelPatch)
	mux.HandleFunc("POST /v1/control/sellers/me/presence", s.handleSellerPresence)
	mux.HandleFunc("PUT /v1/control/sellers/me/ollama", s.handleSellerOllama)

	mux.HandleFunc("POST /v1/control/buyers/me/inference/match", s.handleBuyerInferenceMatch)
	mux.HandleFunc("POST /v1/control/buyers/me/inference/tracking", s.handleBuyerInferenceTracking)
	mux.HandleFunc("POST /v1/control/buyers/me/inference/complete", s.handleBuyerInferenceComplete)
	mux.HandleFunc("POST /v1/control/sellers/me/inference/tracking", s.handleSellerInferenceTracking)
	return mux
}

func (s *HTTPServer) handleBuyerRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body registerReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	id, err := s.Store.RegisterBuyer(body.Name, body.Email, body.Password)
	if err != nil {
		if errors.Is(err, ErrBuyerExists) {
			writeJSONErr(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"buyerId": id,
		"email":   strings.TrimSpace(strings.ToLower(body.Email)),
		"name":    strings.TrimSpace(body.Name),
	})
}

func (s *HTTPServer) handleBuyerLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body loginReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	buyerID, name, email, err := s.Store.LoginBuyer(body.Email, body.Password)
	if err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := IssueBuyerToken(s.Secret, buyerID, email, name)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"accessToken": token,
		"buyerId":     buyerID,
		"name":        name,
		"email":       email,
	})
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
