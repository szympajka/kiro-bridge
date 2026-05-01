package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func flagOrEnv(flagVal, envKey, fallback string) string {
	if flagVal != "" {
		return flagVal
	}
	return env(envKey, fallback)
}

var version = "dev"

// bridgeHolder stores a Bridge that may not be ready yet.
// The handler checks Ready() and returns 503 while connecting.
type bridgeHolder struct {
	mu     sync.RWMutex
	bridge Bridge
}

func (h *bridgeHolder) Set(b Bridge) {
	h.mu.Lock()
	h.bridge = b
	h.mu.Unlock()
}

func (h *bridgeHolder) Get() Bridge {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.bridge
}

var newBridgeFunc = NewBridge

func connectWithBackoff(cfg BridgeConfig, holder *bridgeHolder, stop <-chan struct{}) {
	delay := time.Second
	const maxDelay = 60 * time.Second

	for {
		b, err := newBridgeFunc(cfg)
		if err == nil {
			holder.Set(b)
			log.Printf("bridge connected")
			return
		}
		log.Printf("failed to start bridge: %v (retrying in %s)", err, delay)

		select {
		case <-stop:
			return
		case <-time.After(delay):
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func main() {
	portFlag := flag.String("port", "", "HTTP server port")
	cwdFlag := flag.String("cwd", "", "Working directory for ACP sessions")
	cliFlag := flag.String("cli", "", "Path to kiro-cli binary")
	agentFlag := flag.String("agent", "", "Kiro agent config to activate")
	targetFlag := flag.String("target", "", "Target platform (e.g. claude)")
	flag.Parse()

	port := flagOrEnv(*portFlag, "KIRO_BRIDGE_PORT", "11435")
	cwd := flagOrEnv(*cwdFlag, "KIRO_BRIDGE_CWD", ".")
	cliPath := flagOrEnv(*cliFlag, "KIRO_CLI_PATH", "kiro-cli")
	agent := flagOrEnv(*agentFlag, "KIRO_BRIDGE_AGENT", "kiro-bridge")
	target := flagOrEnv(*targetFlag, "KIRO_BRIDGE_TARGET", "")

	if cwd == "." {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("starting kiro-bridge v%s on 127.0.0.1:%s (cwd=%s, cli=%s, agent=%s, target=%s)", version, port, cwd, cliPath, agent, target)

	loadAgentConfig(agent)

	if target == "claude" {
		claudeMCP := loadClaudeMCPServers(cwd)
		if len(claudeMCP) > 0 {
			log.Printf("loaded %d MCP servers from Claude config", len(claudeMCP))
			mergeClaudeMCPIntoAgent(agent, claudeMCP)
		}
	}

	if enableFS {
		log.Printf("fs/read_text_file and fs/write_text_file enabled")
	}
	if enableTerminal {
		log.Printf("terminal/* methods enabled")
	}

	holder := &bridgeHolder{}
	cfg := BridgeConfig{CLIPath: cliPath, CWD: cwd, Agent: agent, Version: version}

	stop := make(chan struct{})
	go connectWithBackoff(cfg, holder, stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		b := holder.Get()
		if b == nil {
			http.Error(w, "bridge not ready", http.StatusServiceUnavailable)
			return
		}
		handleChatCompletions(b)(w, r)
	})
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		b := holder.Get()
		if b == nil {
			http.Error(w, "bridge not ready", http.StatusServiceUnavailable)
			return
		}
		handleAnthropicMessages(b)(w, r)
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		b := holder.Get()
		if b == nil {
			// Return fallback model when bridge not ready
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"object":"list","data":[{"id":"kiro","object":"model","owned_by":"kiro-bridge"}]}`))
			return
		}
		handleModels(b)(w, r)
	})
	mux.HandleFunc("/healthz", handleHealthz(holder))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		debugf("request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	server := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%s", port), Handler: logMiddleware(mux)}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-sig
	log.Println("shutting down...")
	close(stop)
	server.Shutdown(context.Background())
	if b := holder.Get(); b != nil {
		b.Close()
	}
	log.Println("done")
}

func handleModels(b Bridge) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		models := b.Models()
		if len(models) == 0 {
			w.Write([]byte(`{"object":"list","data":[{"id":"kiro","object":"model","owned_by":"kiro-bridge"}]}`))
			return
		}
		type openaiModel struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		}
		var data []openaiModel
		for _, m := range models {
			data = append(data, openaiModel{ID: m.ID, Object: "model", OwnedBy: "kiro-bridge"})
		}
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	}
}

func handleHealthz(holder *bridgeHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if holder.Get() == nil {
			http.Error(w, "bridge not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	}
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

type stderrWriter struct{ prefix string }

func (w *stderrWriter) Write(p []byte) (int, error) {
	log.Printf("%s%s", w.prefix, bytes.TrimRight(p, "\n"))
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
