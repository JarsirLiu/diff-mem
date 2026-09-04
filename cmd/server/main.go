package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/diff-mem/diff-mem/internal/api"
	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dataDir := flag.String("data", "./diff-mem-data", "storage directory")
	storeKind := flag.String("store", "memory", "storage backend: memory | sqlite | badger")
	flag.Parse()

	var s store.Store
	var err error
	switch *storeKind {
	case "sqlite":
		s, err = store.NewSQLiteStore(*dataDir + "/diff-mem.db")
	case "badger":
		s, err = store.NewFileStore(*dataDir)
	case "memory":
		s = store.NewMemoryStore()
	default:
		log.Fatalf("unknown store backend %q: use memory | sqlite | badger", *storeKind)
	}
	if err != nil {
		log.Fatal("failed to open store: ", err)
	}
	if closer, ok := s.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	e := engine.New(s)
	srv := api.New(e)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	log.Printf("Diff-Mem engine (%s store) listening on %s", *storeKind, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
