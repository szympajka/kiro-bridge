package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var version = "dev"

func main() {
	port := env("KIRO_BRIDGE_PORT", "11435")
	cwd := env("KIRO_BRIDGE_CWD", ".")
	cliPath := env("KIRO_CLI_PATH", "kiro-cli")
	agent := env("KIRO_BRIDGE_AGENT", "kiro-bridge")

	if cwd == "." {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("starting kiro-bridge v%s on 127.0.0.1:%s (cwd=%s, cli=%s, agent=%s)", version, port, cwd, cliPath, agent)

	b, err := NewBridge(BridgeConfig{CLIPath: cliPath, CWD: cwd, Agent: agent, Version: version})
	if err != nil {
		log.Fatalf("failed to start bridge: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions(b))
	mux.HandleFunc("/v1/models", handleModels)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		debugf("request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	server := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%s", port), Handler: logMiddleware(mux)}

	// Graceful shutdown on SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down...")
	server.Shutdown(context.Background())
	b.Close()
	log.Println("done")
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"object":"list","data":[{"id":"kiro","object":"model","owned_by":"kiro-bridge"}]}`))
}

var verboseLog = os.Getenv("KIRO_BRIDGE_VERBOSE") != ""

func debugf(format string, args ...any) {
	if verboseLog {
		log.Printf(format, args...)
	}
}

type debugWriter struct{ prefix string }

func (w *debugWriter) Write(p []byte) (int, error) {
	if verboseLog {
		debugf("%s%s", w.prefix, p)
	}
	return len(p), nil
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if verboseLog {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("error: reading request body: %v", err)
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			log.Printf(">> %s %s\n   Headers: %v\n   Body: %s", r.Method, r.URL.Path, r.Header, body)
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		lw := &loggingWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lw, r)
		if verboseLog {
			log.Printf("<< %s %s %d", r.Method, r.URL.Path, lw.status)
		}
	})
}

type loggingWriter struct {
	http.ResponseWriter
	status int
}

func (lw *loggingWriter) WriteHeader(code int) {
	lw.status = code
	lw.ResponseWriter.WriteHeader(code)
}

func (lw *loggingWriter) Flush() {
	if f, ok := lw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
