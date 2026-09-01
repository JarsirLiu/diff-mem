package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	stdmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/mcp"
	"github.com/diff-mem/diff-mem/internal/store"
)

func main() {
	dataDir := flag.String("data", "./diff-mem-data", "BadgerDB storage directory")
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

	fmt.Fprintln(os.Stderr, "diff-mem MCP server ready")

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		cancel()
	}()

	srv := mcp.New(e)
	if err := srv.Run(ctx, &stdmcp.StdioTransport{}); err != nil {
		log.Fatal("MCP server error: ", err)
	}
}
