package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	stdmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/mcp"
	"github.com/diff-mem/diff-mem/internal/store"
)

func main() {
	port := flag.Int("port", 8787, "SSE listen port (0 to disable SSE)")
	host := flag.String("host", "0.0.0.0", "SSE listen host")
	dataDir := flag.String("data", "./diff-mem-data", "BadgerDB storage directory")
	stdio := flag.Bool("stdio", false, "Use stdio transport (for local MCP clients like Claude Desktop)")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatal(err)
	}

	s, err := store.NewFileStore(*dataDir)
	if err != nil {
		log.Fatal("failed to open store: ", err)
	}
	defer s.Close()

	e := engine.New(s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")
		cancel()
	}()

	srv := mcp.New(e)

	// Default: SSE mode on :8787
	if *stdio {
		fmt.Fprintln(os.Stderr, "diff-mem MCP server ready (stdio)")
		if err := srv.Run(ctx, &stdmcp.StdioTransport{}); err != nil {
			log.Fatal("MCP server error: ", err)
		}
		return
	}

	if *port <= 0 {
		log.Fatal("no transport configured: use -stdio or set -port > 0")
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	mux := http.NewServeMux()
	mux.Handle("/mcp", srv.SSEHandler("/mcp"))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	log.Printf("diff-mem MCP SSE server at http://%s/mcp\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
