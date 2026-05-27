package httpgw

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/auth"
	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/history"
)

//go:embed static/index.html
var staticFS embed.FS

// HealthCheck 是依赖项健康检查函数。
type HealthCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Gateway 是一个 HTTP 通道，通过 REST API 接收消息并返回响应。
type Gateway struct {
	port          int
	msgChan       chan channels.IncomingMessage
	responses     map[string]chan channels.OutgoingResponse // requestID -> response chan
	mu            sync.RWMutex
	server        *http.Server
	shutdown      chan struct{}
	authenticator auth.Authenticator
	historyStore  *history.Store
	pendingStore   *gate.PendingStore
	riskEvaluator  *gate.RiskBasedEvaluator
	healthChecks   []HealthCheck
	startTime      time.Time
	version        string
}

// New 创建新的 HTTP 网关通道。
func New(port int) *Gateway {
	return &Gateway{
		port:      port,
		msgChan:   make(chan channels.IncomingMessage, 64),
		responses: make(map[string]chan channels.OutgoingResponse),
		shutdown:  make(chan struct{}),
		startTime: time.Now(),
		version:   "dev",
	}
}

func (g *Gateway) Name() string { return "http" }

// WithAuth 设置认证器。
func (g *Gateway) WithAuth(a auth.Authenticator) *Gateway {
	g.authenticator = a
	return g
}

// WithHistory 设置 history store 以支持历史查询端点。
func (g *Gateway) WithHistory(h *history.Store) *Gateway {
	g.historyStore = h
	return g
}

// WithPendingStore 设置 gate pending store 以支持审批端点。
func (g *Gateway) WithPendingStore(ps *gate.PendingStore) *Gateway {
	g.pendingStore = ps
	return g
}

// WithRiskEvaluator 设置风险策略评估器以支持学习模式。
func (g *Gateway) WithRiskEvaluator(ev *gate.RiskBasedEvaluator) *Gateway {
	g.riskEvaluator = ev
	return g
}

// WithVersion 设置服务版本号。
func (g *Gateway) WithVersion(v string) *Gateway {
	g.version = v
	return g
}

// RegisterHealthCheck 注册依赖项健康检查。
func (g *Gateway) RegisterHealthCheck(name string, check func(ctx context.Context) error) {
	g.healthChecks = append(g.healthChecks, HealthCheck{Name: name, Check: check})
}

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
	mux.HandleFunc("/api/history/threads", g.handleHistoryThreads)
	mux.HandleFunc("/api/history/threads/", g.handleHistoryThreadDetail)
	mux.HandleFunc("/api/history/stats", g.handleHistoryStats)
	mux.HandleFunc("/api/gates/pending", g.handleGatesPending)
	mux.HandleFunc("/api/gates/approve", g.handleGateApprove)
	mux.HandleFunc("/api/gates/deny", g.handleGateDeny)
	mux.HandleFunc("/health", g.handleHealth)
	mux.HandleFunc("/readyz", g.handleReadyz)
	mux.HandleFunc("/livez", g.handleLivez)
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

	// 认证：若提供了 API Key，则通过认证器解析用户身份；否则保留前端传入的 user_id
	if g.authenticator != nil && req.APIKey != "" {
		userID, err := g.authenticator.Authenticate(r.Context(), req.APIKey)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		req.UserID = userID
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

	// 处理附件
	var attachments []channels.Attachment
	for _, attReq := range req.Attachments {
		data, _ := base64.StdEncoding.DecodeString(attReq.DataBase64)
		attachments = append(attachments, channels.Attachment{
			Kind:     channels.AttachmentKindDocument,
			MIMEType: attReq.MIMEType,
			Filename: attReq.Filename,
			SizeBytes: int64(len(data)),
			Data:      data,
		})
	}

	// 将消息发送到 Agent
	g.msgChan <- channels.IncomingMessage{
		ID:          requestID,
		Channel:     g.Name(),
		UserID:      req.UserID,
		Content:     req.Content,
		ThreadID:    req.ThreadID,
		Attachments: attachments,
	}

	// 等待响应（带超时）
	select {
	case resp := <-respChan:
		w.Header().Set("Content-Type", "application/json")
		if resp.Status == "pending_gate" {
			var gateInfo map[string]any
			if g.pendingStore != nil {
				if pg, ok := g.pendingStore.Get(req.UserID, resp.ThreadID); ok {
					gateInfo = map[string]any{
						"request_id":  pg.RequestID,
						"tool_name":   pg.ToolName,
						"description": pg.Description,
					}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":    "pending_gate",
				"thread_id": resp.ThreadID,
				"gate":      gateInfo,
			})
			return
		}
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
	g.mu.RLock()
	checkCount := len(g.healthChecks)
	g.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"channel":        g.Name(),
		"version":        g.version,
		"uptime_seconds": int(time.Since(g.startTime).Seconds()),
		"health_checks":  checkCount,
	})
}

func (g *Gateway) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	g.mu.RLock()
	checks := make([]HealthCheck, len(g.healthChecks))
	copy(checks, g.healthChecks)
	g.mu.RUnlock()

	var failed []map[string]any
	for _, hc := range checks {
		if err := hc.Check(ctx); err != nil {
			failed = append(failed, map[string]any{
				"name":   hc.Name,
				"status": "failed",
				"error":  err.Error(),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if len(failed) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "not_ready",
			"failed":  failed,
			"checked": len(checks),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ready",
		"checked": len(checks),
	})
}

func (g *Gateway) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "alive",
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

type attachmentRequest struct {
	MIMEType   string `json:"mime_type"`
	Filename   string `json:"filename"`
	DataBase64 string `json:"data_base64"`
}

type chatRequest struct {
	UserID      string              `json:"user_id"`
	Content     string              `json:"content"`
	ThreadID    string              `json:"thread_id,omitempty"`
	APIKey      string              `json:"api_key,omitempty"`
	Attachments []attachmentRequest `json:"attachments,omitempty"`
}

type chatResponse struct {
	Content  string `json:"content"`
	ThreadID string `json:"thread_id,omitempty"`
}

func (g *Gateway) handleHistoryThreads(w http.ResponseWriter, r *http.Request) {
	if g.historyStore == nil {
		http.Error(w, `{"error":"history not available"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := g.authenticatedUserID(r)
	if userID == "" {
		userID = "anonymous"
	}

	convs, err := g.historyStore.UserConversations(r.Context(), userID, 100, 0)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"threads": convs,
		"count":   len(convs),
	})
}

func (g *Gateway) handleHistoryThreadDetail(w http.ResponseWriter, r *http.Request) {
	if g.historyStore == nil {
		http.Error(w, `{"error":"history not available"}`, http.StatusServiceUnavailable)
		return
	}

	prefix := "/api/history/threads/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	threadID := strings.TrimPrefix(r.URL.Path, prefix)
	if threadID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		msgs, err := g.historyStore.ThreadHistory(r.Context(), threadID, 1000, 0)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"thread_id": threadID,
			"messages":  msgs,
			"count":     len(msgs),
		})
	case http.MethodDelete:
		if err := g.historyStore.DeleteThread(r.Context(), threadID); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) handleHistoryStats(w http.ResponseWriter, r *http.Request) {
	if g.historyStore == nil {
		http.Error(w, `{"error":"history not available"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := g.historyStore.GetJobStats(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	toolStats, err := g.historyStore.GetToolStats(r.Context())
	if err != nil {
		toolStats = nil
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jobs":  stats,
		"tools": toolStats,
	})
}

func (g *Gateway) authenticatedUserID(r *http.Request) string {
	if g.authenticator == nil {
		return ""
	}
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
	}
	userID, err := g.authenticator.Authenticate(r.Context(), apiKey)
	if err != nil {
		return ""
	}
	return userID
}

func (g *Gateway) handleGatesPending(w http.ResponseWriter, r *http.Request) {
	if g.pendingStore == nil {
		http.Error(w, `{"error":"gate pending store not available"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 免认证模式下，允许前端通过 query param 传递 user_id 以过滤自己的审批项
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = g.authenticatedUserID(r)
	}
	if userID == "" {
		userID = "anonymous"
	}

	all := g.pendingStore.List()
	var filtered []map[string]any
	for _, pg := range all {
		if pg.UserID == userID {
			filtered = append(filtered, map[string]any{
				"request_id":     pg.RequestID,
				"tool_name":      pg.ToolName,
				"description":    pg.Description,
				"created_at":     pg.CreatedAt,
				"expires_at":     pg.ExpiresAt,
				"thread_id":      pg.ThreadID,
				"source_channel": pg.SourceChannel,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"gates": filtered,
		"count": len(filtered),
	})
}

func (g *Gateway) handleGateApprove(w http.ResponseWriter, r *http.Request) {
	if g.pendingStore == nil {
		http.Error(w, `{"error":"gate pending store not available"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RequestID string `json:"request_id"`
		UserID    string `json:"user_id"`
		ThreadID  string `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid json: %v"}`, err), http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		req.UserID = g.authenticatedUserID(r)
	}
	if req.UserID == "" {
		req.UserID = "anonymous"
	}
	if req.ThreadID == "" {
		http.Error(w, `{"error":"thread_id is required"}`, http.StatusBadRequest)
		return
	}

	pg, err := g.pendingStore.Resolve(req.UserID, req.ThreadID, req.RequestID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusNotFound)
		return
	}

	// pg == nil 表示幂等：该请求已被处理过（已消费）
	toolName := ""
	if pg != nil {
		toolName = pg.ToolName
		// 学习模式：记录用户审批
		if g.riskEvaluator != nil {
			g.riskEvaluator.RecordApproval(pg.ToolName, pg.Params, req.UserID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"approved":   true,
		"request_id": req.RequestID,
		"tool_name":  toolName,
	})
}

func (g *Gateway) handleGateDeny(w http.ResponseWriter, r *http.Request) {
	if g.pendingStore == nil {
		http.Error(w, `{"error":"gate pending store not available"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RequestID string `json:"request_id"`
		UserID    string `json:"user_id"`
		ThreadID  string `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid json: %v"}`, err), http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		req.UserID = g.authenticatedUserID(r)
	}
	if req.UserID == "" {
		req.UserID = "anonymous"
	}
	if req.ThreadID == "" {
		http.Error(w, `{"error":"thread_id is required"}`, http.StatusBadRequest)
		return
	}

	if err := g.pendingStore.Deny(req.UserID, req.ThreadID, req.RequestID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"denied":     true,
		"request_id": req.RequestID,
	})
}
