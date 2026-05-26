package agent

import (
	"testing"
	"time"
)

func TestSessionManagerGetOrCreate(t *testing.T) {
	sm := NewSessionManager()

	thread := sm.GetOrCreateThread("user1", "repl")
	if thread.ID == "" {
		t.Fatal("expected thread ID")
	}
	if thread.UserID != "user1" {
		t.Errorf("userID = %q, want user1", thread.UserID)
	}

	// Same user should return same thread
	thread2 := sm.GetOrCreateThread("user1", "repl")
	if thread2.ID != thread.ID {
		t.Error("expected same thread for same user")
	}

	// Different user should return different thread
	thread3 := sm.GetOrCreateThread("user2", "repl")
	if thread3.ID == thread.ID {
		t.Error("expected different thread for different user")
	}
}

func TestSessionManagerAddTurn(t *testing.T) {
	sm := NewSessionManager()
	sm.GetOrCreateThread("user1", "repl")

	sm.AddTurn("user1", Turn{UserMsg: "hi", AgentResp: "hello"})
	turns := sm.GetTurns("user1")
	if len(turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(turns))
	}
	if turns[0].UserMsg != "hi" {
		t.Errorf("user msg = %q", turns[0].UserMsg)
	}

	sm.AddTurn("user1", Turn{UserMsg: "bye", AgentResp: "goodbye"})
	turns = sm.GetTurns("user1")
	if len(turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(turns))
	}
}

func TestSessionManagerGetTurnsMissingUser(t *testing.T) {
	sm := NewSessionManager()
	turns := sm.GetTurns("nobody")
	if turns != nil {
		t.Errorf("expected nil, got %v", turns)
	}
}

func TestSessionManagerSwitchThread(t *testing.T) {
	sm := NewSessionManager()
	sm.GetOrCreateThread("user1", "repl")

	// Create a second thread manually
	t2 := &Thread{
		ID:        "thread-2",
		UserID:    "user1",
		Channel:   "repl",
		Title:     "Second",
		CreatedAt: time.Now(),
	}
	sm.mu.Lock()
	sm.history["user1"] = append(sm.history["user1"], t2)
	sm.mu.Unlock()

	if ok := sm.SwitchThread("user1", "thread-2"); !ok {
		t.Fatal("expected switch to succeed")
	}

	active := sm.GetOrCreateThread("user1", "repl")
	if active.ID != "thread-2" {
		t.Errorf("active thread = %q, want thread-2", active.ID)
	}

	// Switch to non-existent thread
	if ok := sm.SwitchThread("user1", "missing"); ok {
		t.Error("expected switch to missing thread to fail")
	}
}

func TestSessionManagerListThreads(t *testing.T) {
	sm := NewSessionManager()
	sm.GetOrCreateThread("user1", "repl")

	t2 := &Thread{ID: "t2", UserID: "user1", Title: "Second"}
	sm.mu.Lock()
	sm.history["user1"] = append(sm.history["user1"], t2)
	sm.mu.Unlock()

	threads := sm.ListThreads("user1")
	if len(threads) != 2 {
		t.Errorf("threads = %d, want 2", len(threads))
	}
}

func TestSessionManagerCompactThread(t *testing.T) {
	sm := NewSessionManager()
	sm.GetOrCreateThread("user1", "repl")

	for i := 0; i < 10; i++ {
		sm.AddTurn("user1", Turn{UserMsg: "msg"})
	}

	sm.CompactThread("user1", 5)
	turns := sm.GetTurns("user1")
	if len(turns) != 5 {
		t.Errorf("turns = %d, want 5", len(turns))
	}

	// Compacting again with same limit should be no-op
	sm.CompactThread("user1", 5)
	turns = sm.GetTurns("user1")
	if len(turns) != 5 {
		t.Errorf("turns = %d, want 5 after no-op", len(turns))
	}
}

func TestSessionManagerGetThread(t *testing.T) {
	sm := NewSessionManager()
	t1 := sm.GetOrCreateThread("user1", "repl")

	got := sm.GetThread("user1", t1.ID)
	if got == nil || got.ID != t1.ID {
		t.Error("expected to find thread")
	}

	if got := sm.GetThread("user1", "missing"); got != nil {
		t.Error("expected nil for missing thread")
	}
}
