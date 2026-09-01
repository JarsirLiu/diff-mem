package main

import (
	"log"
	"net/http"

	"github.com/diff-mem/diff-mem/internal/api"
	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/store"
)

func main() {
	s := store.NewMemoryStore()
	e := engine.New(s)
	srv := api.New(e)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	addr := ":8080"
	log.Printf("Diff-Mem engine listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
