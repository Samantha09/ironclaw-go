package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/channels"
)

// Server 提供 Webhook 接收服务。
type Server struct {
	port     int
	msgChan  chan channels.IncomingMessage
	server   *http.Server
	shutdown chan struct{}
}

// NewServer 创建新的 Webhook 服务器。
func NewServer(port int) *Server {
	return &Server{
		port:     port,
		msgChan:  make(chan channels.IncomingMessage, 64),
		shutdown: make(chan struct{}),
	}
}

func (s *Server) Name() string { return "webhook" }

func (s *Server) Messages() <-chan channels.IncomingMessage {
	return s.msgChan
}

func (s *Server) SendMessage(_ context.Context, msg channels.OutgoingResponse) error {
	// Webhook 是单向输入通道，不发送响应
	_ = msg
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	close(s.shutdown)
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Start 启动 Webhook HTTP 服务器。
func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	mux.HandleFunc("/webhook/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("webhook server error: %v\n", err)
		}
	}()
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	// 提取用户 ID 和内容
	userID := "webhook"
	if u, ok := payload["user_id"].(string); ok && u != "" {
		userID = u
	}

	content := ""
	if c, ok := payload["content"].(string); ok {
		content = c
	} else {
		// 将整个 payload 作为内容
		b, _ := json.Marshal(payload)
		content = string(b)
	}

	select {
	case s.msgChan <- channels.IncomingMessage{
		ID:      uuid.New().String(),
		Channel: s.Name(),
		UserID:  userID,
		Content: content,
	}:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	case <-time.After(2 * time.Second):
		http.Error(w, `{"error":"channel full"}`, http.StatusServiceUnavailable)
	case <-s.shutdown:
		http.Error(w, `{"error":"server shutting down"}`, http.StatusServiceUnavailable)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"channel": s.Name(),
	})
}
