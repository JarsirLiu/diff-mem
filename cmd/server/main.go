package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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
		s, err = store.NewSQLiteStore(filepath.Join(*dataDir, "diff-mem.db"))
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")
		cancel()
	}()

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("Diff-Mem engine (%s store) listening on %s", *storeKind, *addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error: ", err)
	}
}
