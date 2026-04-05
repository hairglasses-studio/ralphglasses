package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/runmylife/internal/api"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/db"
	"github.com/hairglasses-studio/runmylife/internal/events"
	"github.com/hairglasses-studio/runmylife/internal/mcp"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

func main() {
	transport := flag.String("transport", "", "Transport mode: stdio (default), sse, or http")
	port := flag.String("port", "", "Port for SSE/HTTP server (default 8080)")
	flag.Parse()

	// CLI flags take priority, then env vars
	mode := *transport
	if mode == "" {
		mode = os.Getenv("MCP_MODE")
	}
	ssePort := *port
	if ssePort == "" {
		ssePort = os.Getenv("PORT")
	}
	if ssePort == "" {
		ssePort = "8080"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("[runmylife-mcp] Shutting down...")
		cancel()
		<-sigChan
		os.Exit(1)
	}()

	hooks := mcp.ConfigureHooks()

	s := server.NewMCPServer("runmylife", "0.1.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithHooks(hooks),
	)

	mcp.RegisterTools(s)
	mcp.RegisterResources(s)
	mcp.RegisterPrompts(s)

	// Open DB + event bus for REST API (only used in sse/http modes)
	var openAPIDB = func() (*db.DB, *events.Bus) {
		cfg, err := config.Load()
		if err != nil {
			log.Printf("[runmylife-mcp] Config load for API: %v (REST API disabled)", err)
			return nil, nil
		}
		database, err := db.Open(cfg.DBPath)
		if err != nil {
			log.Printf("[runmylife-mcp] DB open for API: %v (REST API disabled)", err)
			return nil, nil
		}
		bus := events.NewBus(database.SqlDB())
		common.SetBus(bus)
		return database, bus
	}

	switch mode {
	case "sse":
		log.Printf("[runmylife-mcp] Starting SSE server on :%s", ssePort)

		sseServer := server.NewSSEServer(s)
		mux := http.NewServeMux()
		mux.Handle("/sse", sseServer)
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})

		if database, bus := openAPIDB(); database != nil {
			api.MountRoutes(mux, database.SqlDB(), bus, events.NewEmitter(bus), os.Getenv("RML_API_TOKEN"))
			log.Println("[runmylife-mcp] REST API mounted on /api/v1/")
		}

		srv := &http.Server{Addr: ":" + ssePort, Handler: mux}
		go func() {
			<-ctx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("[runmylife-mcp] SSE server error: %v", err)
		}

	case "http":
		log.Printf("[runmylife-mcp] Starting Streamable HTTP server on :%s/mcp", ssePort)

		httpServer := server.NewStreamableHTTPServer(s)
		mux := http.NewServeMux()
		mux.Handle("/mcp", httpServer)
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})

		if database, bus := openAPIDB(); database != nil {
			api.MountRoutes(mux, database.SqlDB(), bus, events.NewEmitter(bus), os.Getenv("RML_API_TOKEN"))
			log.Println("[runmylife-mcp] REST API mounted on /api/v1/")
		}

		srv := &http.Server{Addr: ":" + ssePort, Handler: mux}
		go func() {
			<-ctx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("[runmylife-mcp] Streamable HTTP server error: %v", err)
		}

	default:
		log.Println("[runmylife-mcp] Starting stdio server")
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("[runmylife-mcp] Stdio server error: %v", err)
		}
	}
}
