package httpgw

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/channels"
)

//go:embed static/index.html
var staticFS embed.FS

// Gateway 是一个 HTTP 通道，通过 REST API 接收消息并返回响应。
type Gateway struct {
	port      int
	msgChan   chan channels.IncomingMessage
	responses map[string]chan channels.OutgoingResponse // requestID -> response chan
	mu        sync.RWMutex
	server    *http.Server
	shutdown  chan struct{}
}

// New 创建新的 HTTP 网关通道。
func New(port int) *Gateway {
	return &Gateway{
		port:      port,
		msgChan:   make(chan channels.IncomingMessage, 64),
		responses: make(map[string]chan channels.OutgoingResponse),
		shutdown:  make(chan struct{}),
	}
}

func (g *Gateway) Name() string { return "http" }

func (g *Gateway) Messages() <-chan channels.IncomingMessage {
	return g.msgChan
}

func (g *Gateway) SendMessage(_ context.Context, msg channels.OutgoingResponse) error {
	// Broadcast 会调用此方法，将响应路由到等待的 HTTP 请求
	// 简化：使用 ThreadID 或 UserID 匹配
	// MVP: 广播给所有等待的请求
	g.mu.RLock()
	chans := make([]chan channels.OutgoingResponse, 0, len(g.responses))
	for _, ch := range g.responses {
		chans = append(chans, ch)
	}
	g.mu.RUnlock()

	for _, ch := range chans {
		select {
		case ch <- msg:
		default:
		}
	}
	return nil
}

func (g *Gateway) Shutdown(ctx context.Context) error {
	close(g.shutdown)
	if g.server != nil {
		return g.server.Shutdown(ctx)
	}
	return nil
}

// Start 启动 HTTP 服务器。
func (g *Gateway) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", g.handleChat)
	mux.HandleFunc("/health", g.handleHealth)
	mux.HandleFunc("/", g.handleIndex)

	g.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", g.port),
		Handler: mux,
	}

	go func() {
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("http gateway error: %v\n", err)
		}
	}()
}

func (g *Gateway) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		req.UserID = "anonymous"
	}
	if req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	requestID := uuid.New().String()
	respChan := make(chan channels.OutgoingResponse, 1)

	g.mu.Lock()
	g.responses[requestID] = respChan
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.responses, requestID)
		g.mu.Unlock()
	}()

	// 将消息发送到 Agent
	g.msgChan <- channels.IncomingMessage{
		ID:       requestID,
		Channel:  g.Name(),
		UserID:   req.UserID,
		Content:  req.Content,
		ThreadID: req.ThreadID,
	}

	// 等待响应（带超时）
	select {
	case resp := <-respChan:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatResponse{
			Content:  resp.Content,
			ThreadID: resp.ThreadID,
		})
	case <-time.After(30 * time.Second):
		http.Error(w, `{"error":"timeout"}`, http.StatusGatewayTimeout)
	case <-g.shutdown:
		http.Error(w, `{"error":"server shutting down"}`, http.StatusServiceUnavailable)
	}
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"channel": g.Name(),
	})
}

func (g *Gateway) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

type chatRequest struct {
	UserID   string `json:"user_id"`
	Content  string `json:"content"`
	ThreadID string `json:"thread_id,omitempty"`
}

type chatResponse struct {
	Content  string `json:"content"`
	ThreadID string `json:"thread_id,omitempty"`
}
