package api

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	Addr string
}

func NewServer(addr string) *Server {
	return &Server{Addr: addr}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"module": "knowledgeMesh",
		})
	})
	return mux
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.Addr, s.Handler())
}
