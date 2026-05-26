package gate

import (
	"testing"
	"time"
)

func TestPendingStoreCreateAndGet(t *testing.T) {
	s := NewPendingStore()

	pg := s.Create("user1", "thread1", "shell", map[string]any{"command": "ls"}, "repl")
	if pg.RequestID == "" {
		t.Fatal("expected request ID")
	}
	if pg.ToolName != "shell" {
		t.Errorf("tool = %q, want shell", pg.ToolName)
	}
	if pg.Description == "" {
		t.Error("expected description")
	}

	// Get by user/thread
	got, ok := s.Get("user1", "thread1")
	if !ok {
		t.Fatal("expected to find pending gate")
	}
	if got.RequestID != pg.RequestID {
		t.Errorf("request ID mismatch")
	}

	// Get by request ID
	got2, ok := s.GetByRequestID(pg.RequestID)
	if !ok {
		t.Fatal("expected to find by request ID")
	}
	if got2.RequestID != pg.RequestID {
		t.Errorf("request ID mismatch")
	}
}

func TestPendingStoreResolve(t *testing.T) {
	s := NewPendingStore()
	pg := s.Create("user1", "thread1", "shell", nil, "repl")

	resolved, err := s.Resolve("user1", "thread1", pg.RequestID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.RequestID != pg.RequestID {
		t.Error("resolved wrong gate")
	}

	// Should be removed
	if _, ok := s.Get("user1", "thread1"); ok {
		t.Error("expected gate to be removed after resolve")
	}
}

func TestPendingStoreResolveMismatch(t *testing.T) {
	s := NewPendingStore()
	s.Create("user1", "thread1", "shell", nil, "repl")

	_, err := s.Resolve("user1", "thread1", "wrong-id")
	if err == nil {
		t.Error("expected error for request ID mismatch")
	}
}

func TestPendingStoreResolveNotFound(t *testing.T) {
	s := NewPendingStore()
	_, err := s.Resolve("user1", "thread1", "id")
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestPendingStoreDeny(t *testing.T) {
	s := NewPendingStore()
	pg := s.Create("user1", "thread1", "shell", nil, "repl")

	if err := s.Deny("user1", "thread1", pg.RequestID); err != nil {
		t.Fatalf("deny: %v", err)
	}

	if _, ok := s.Get("user1", "thread1"); ok {
		t.Error("expected gate to be removed after deny")
	}
}

func TestPendingStoreExpired(t *testing.T) {
	s := NewPendingStore()
	s.ttl = 1 * time.Millisecond

	pg := s.Create("user1", "thread1", "shell", nil, "repl")
	time.Sleep(10 * time.Millisecond)

	if _, ok := s.Get("user1", "thread1"); ok {
		t.Error("expected expired gate to be removed")
	}

	if _, ok := s.GetByRequestID(pg.RequestID); ok {
		t.Error("expected expired gate to be removed by request ID")
	}
}

func TestPendingStoreList(t *testing.T) {
	s := NewPendingStore()
	s.Create("user1", "thread1", "shell", nil, "repl")
	s.Create("user2", "thread2", "file", nil, "repl")

	list := s.List()
	if len(list) != 2 {
		t.Errorf("list = %d, want 2", len(list))
	}
}

func TestPendingStoreCount(t *testing.T) {
	s := NewPendingStore()
	if s.Count() != 0 {
		t.Errorf("count = %d, want 0", s.Count())
	}
	s.Create("user1", "thread1", "shell", nil, "repl")
	if s.Count() != 1 {
		t.Errorf("count = %d, want 1", s.Count())
	}
}

func TestPendingStoreGetByRequestIDNotFound(t *testing.T) {
	s := NewPendingStore()
	if _, ok := s.GetByRequestID("nonexistent"); ok {
		t.Error("expected not found")
	}
}

func TestPendingStoreOverwrite(t *testing.T) {
	s := NewPendingStore()
	pg1 := s.Create("user1", "thread1", "shell", nil, "repl")
	pg2 := s.Create("user1", "thread1", "file", nil, "repl")

	got, ok := s.Get("user1", "thread1")
	if !ok {
		t.Fatal("expected to find")
	}
	if got.RequestID == pg1.RequestID {
		t.Error("expected old gate to be overwritten")
	}
	if got.RequestID != pg2.RequestID {
		t.Error("expected new gate")
	}
}
