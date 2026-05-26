// Package websocket 提供 WebSocket 通道实现。
package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/auth"
	"github.com/nearai/ironclaw-go/internal/channels"
	"golang.org/x/net/websocket"
)

// wsMessage 是客户端与服务器之间的消息格式。
type wsMessage struct {
	UserID   string `json:"user_id,omitempty"`
	Content  string `json:"content"`
	ThreadID string `json:"thread_id,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

// Channel 是 WebSocket 通道实现。
type Channel struct {
	port          int
	msgChan       chan channels.IncomingMessage
	clients       map[string]*websocket.Conn // connID -> conn
	mu            sync.RWMutex
	server        *http.Server
	shutdown      chan struct{}
	authenticator auth.Authenticator
}

// New 创建新的 WebSocket 通道。
func New(port int) *Channel {
	return &Channel{
		port:     port,
		msgChan:  make(chan channels.IncomingMessage, 64),
		clients:  make(map[string]*websocket.Conn),
		shutdown: make(chan struct{}),
	}
}

func (c *Channel) Name() string { return "websocket" }

// WithAuth 设置认证器。
func (c *Channel) WithAuth(a auth.Authenticator) *Channel {
	c.authenticator = a
	return c
}

func (c *Channel) Messages() <-chan channels.IncomingMessage {
	return c.msgChan
}

// SendMessage 向所有已连接的 WebSocket 客户端广播响应。
func (c *Channel) SendMessage(_ context.Context, msg channels.OutgoingResponse) error {
	payload, err := json.Marshal(wsMessage{
		Content:  msg.Content,
		ThreadID: msg.ThreadID,
	})
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	c.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(c.clients))
	for _, conn := range c.clients {
		conns = append(conns, conn)
	}
	c.mu.RUnlock()

	for _, conn := range conns {
		if _, err := conn.Write(payload); err != nil {
			// 静默忽略写入失败的连接，由读取循环处理断开
			continue
		}
	}
	return nil
}

// Shutdown 关闭 WebSocket 服务器和所有连接。
func (c *Channel) Shutdown(ctx context.Context) error {
	close(c.shutdown)

	c.mu.Lock()
	for id, conn := range c.clients {
		conn.Close()
		delete(c.clients, id)
	}
	c.mu.Unlock()

	if c.server != nil {
		return c.server.Shutdown(ctx)
	}
	return nil
}

// Start 启动 WebSocket HTTP 服务器。
func (c *Channel) Start() {
	mux := http.NewServeMux()
	mux.Handle("/ws", websocket.Handler(c.handleConn))
	mux.HandleFunc("/health", c.handleHealth)

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.port),
		Handler: mux,
	}

	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("websocket server error: %v\n", err)
		}
	}()
}

func (c *Channel) handleConn(conn *websocket.Conn) {
	connID := uuid.New().String()

	c.mu.Lock()
	c.clients[connID] = conn
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.clients, connID)
		c.mu.Unlock()
		conn.Close()
	}()

	for {
		select {
		case <-c.shutdown:
			return
		default:
		}

		var msg wsMessage
		if err := websocket.JSON.Receive(conn, &msg); err != nil {
			// 连接断开或解析错误
			return
		}

		if msg.Content == "" {
			continue
		}

		userID := msg.UserID
		if c.authenticator != nil {
			authUserID, err := c.authenticator.Authenticate(context.Background(), msg.APIKey)
			if err != nil {
				// 认证失败，发送错误并关闭连接
				_ = websocket.JSON.Send(conn, wsMessage{Content: "unauthorized"})
				return
			}
			userID = authUserID
		}
		if userID == "" {
			userID = connID
		}

		requestID := uuid.New().String()
		c.msgChan <- channels.IncomingMessage{
			ID:       requestID,
			Channel:  c.Name(),
			UserID:   userID,
			Content:  msg.Content,
			ThreadID: msg.ThreadID,
		}
	}
}

func (c *Channel) handleHealth(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	clientCount := len(c.clients)
	c.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":       "ok",
		"channel":      c.Name(),
		"clients":      clientCount,
		"ws_endpoint":  fmt.Sprintf("ws://localhost:%d/ws", c.port),
	})
}
