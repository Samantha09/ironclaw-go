package httpgw

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nearai/ironclaw-go/internal/channels"
	"github.com/nearai/ironclaw-go/internal/gate"
)

func TestGatewayHealth(t *testing.T) {
	gw := New(0)
	gw.WithVersion("test-v1")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	gw.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("expected ok status, got %s", body)
	}
	if !strings.Contains(body, `"version":"test-v1"`) {
		t.Errorf("expected version, got %s", body)
	}
}

func TestGatewayLivez(t *testing.T) {
	gw := New(0)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	gw.handleLivez(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"alive"`) {
		t.Errorf("expected alive status, got %s", body)
	}
}

func TestGatewayReadyzAllPass(t *testing.T) {
	gw := New(0)
	gw.RegisterHealthCheck("db", func(ctx context.Context) error { return nil })
	gw.RegisterHealthCheck("llm", func(ctx context.Context) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	gw.handleReadyz(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"ready"`) {
		t.Errorf("expected ready status, got %s", body)
	}
}

func TestGatewayReadyzOneFails(t *testing.T) {
	gw := New(0)
	gw.RegisterHealthCheck("db", func(ctx context.Context) error { return nil })
	gw.RegisterHealthCheck("llm", func(ctx context.Context) error { return fmt.Errorf("timeout") })

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	gw.handleReadyz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"not_ready"`) {
		t.Errorf("expected not_ready status, got %s", body)
	}
	if !strings.Contains(body, "timeout") {
		t.Errorf("expected error message, got %s", body)
	}
}

func TestGatewaySendMessage(t *testing.T) {
	gw := New(0)
	ctx := context.Background()

	// 发送消息到等待的响应通道
	resp := channels.OutgoingResponse{Content: "hello", ThreadID: "t1"}
	if err := gw.SendMessage(ctx, resp); err != nil {
		t.Fatalf("send: %v", err)
	}
	// 没有等待的请求，不应 panic
}

func TestGatewayAuthenticatedUserID(t *testing.T) {
	gw := New(0)

	// 无认证器
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	if id := gw.authenticatedUserID(req); id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

func TestGatewayStartAndShutdown(t *testing.T) {
	gw := New(18080)
	gw.Start()

	// 等待服务器启动
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:18080/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := gw.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestGatewayGatesPending(t *testing.T) {
	gw := New(0)
	ps := gate.NewPendingStore()
	gw.WithPendingStore(ps)
	gw.authenticator = &mockAuth{userID: "user1"}

	ps.Create("user1", "thread1", "shell", map[string]any{"command": "ls"}, "http")
	ps.Create("user2", "thread2", "file", map[string]any{"action": "write"}, "http")

	req := httptest.NewRequest(http.MethodGet, "/api/gates/pending", nil)
	w := httptest.NewRecorder()
	gw.handleGatesPending(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "shell") {
		t.Errorf("expected shell gate, got %s", body)
	}
	if strings.Contains(body, "file") {
		t.Errorf("did not expect file gate for other user, got %s", body)
	}
}

func TestGatewayGatesPendingNoStore(t *testing.T) {
	gw := New(0)
	req := httptest.NewRequest(http.MethodGet, "/api/gates/pending", nil)
	w := httptest.NewRecorder()
	gw.handleGatesPending(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestGatewayGateApprove(t *testing.T) {
	gw := New(0)
	ps := gate.NewPendingStore()
	gw.WithPendingStore(ps)

	pg := ps.Create("user1", "thread1", "shell", nil, "http")

	req := httptest.NewRequest(http.MethodPost, "/api/gates/approve", strings.NewReader(fmt.Sprintf(`{
		"request_id": "%s",
		"user_id": "user1",
		"thread_id": "thread1"
	}`, pg.RequestID)))
	w := httptest.NewRecorder()
	gw.handleGateApprove(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"approved":true`) {
		t.Errorf("expected approved=true, got %s", w.Body.String())
	}

	// Should be removed
	if _, ok := ps.Get("user1", "thread1"); ok {
		t.Error("expected gate to be removed after approve")
	}
}

func TestGatewayGateApproveMismatch(t *testing.T) {
	gw := New(0)
	ps := gate.NewPendingStore()
	gw.WithPendingStore(ps)

	ps.Create("user1", "thread1", "shell", nil, "http")

	req := httptest.NewRequest(http.MethodPost, "/api/gates/approve", strings.NewReader(`{
		"request_id": "wrong-id",
		"user_id": "user1",
		"thread_id": "thread1"
	}`))
	w := httptest.NewRecorder()
	gw.handleGateApprove(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestGatewayGateDeny(t *testing.T) {
	gw := New(0)
	ps := gate.NewPendingStore()
	gw.WithPendingStore(ps)

	pg := ps.Create("user1", "thread1", "shell", nil, "http")

	req := httptest.NewRequest(http.MethodPost, "/api/gates/deny", strings.NewReader(fmt.Sprintf(`{
		"request_id": "%s",
		"user_id": "user1",
		"thread_id": "thread1"
	}`, pg.RequestID)))
	w := httptest.NewRecorder()
	gw.handleGateDeny(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"denied":true`) {
		t.Errorf("expected denied=true, got %s", w.Body.String())
	}
}

func TestGatewayGateDenyMissingThreadID(t *testing.T) {
	gw := New(0)
	ps := gate.NewPendingStore()
	gw.WithPendingStore(ps)

	req := httptest.NewRequest(http.MethodPost, "/api/gates/deny", strings.NewReader(`{
		"request_id": "id1"
	}`))
	w := httptest.NewRecorder()
	gw.handleGateDeny(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

type mockAuth struct {
	userID string
}

func (m *mockAuth) Authenticate(_ context.Context, _ string) (string, error) {
	return m.userID, nil
}
