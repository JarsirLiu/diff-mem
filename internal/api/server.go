// Package api provides the HTTP server that exposes Diff-Mem tools.
package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/model"
)

type Server struct {
	engine *engine.Engine
}

func New(e *engine.Engine) *Server {
	return &Server{engine: e}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// POST /tools/{name} — main tool invocation endpoint
	mux.HandleFunc("POST /tools/create", s.handle("create"))
	mux.HandleFunc("POST /tools/append", s.handle("append"))
	mux.HandleFunc("POST /tools/update", s.handle("update"))
	mux.HandleFunc("POST /tools/lifecycle", s.handle("lifecycle"))
	mux.HandleFunc("POST /tools/list", s.handle("list"))
	mux.HandleFunc("POST /tools/search", s.handle("search"))
	mux.HandleFunc("POST /tools/show", s.handle("show"))
	mux.HandleFunc("POST /tools/exec", s.handleExec)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}

func (s *Server) handle(toolName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var params map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PARAMS", err.Error())
			return
		}

		resp := s.engine.Dispatch(toolName, params)
		writeJSON(w, resp)
	}
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Operations []map[string]interface{} `json:"operations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", err.Error())
		return
	}
	resp := s.engine.Exec(body.Operations)
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	resp := model.ToolResponse{
		Success: false,
		Error: &model.ErrorInfo{Code: code, Message: message},
	}
	json.NewEncoder(w).Encode(resp)
}

func init() {
	_ = log.Println // keep log imported
}
