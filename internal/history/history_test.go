package history

import (
	"context"
	"testing"
	"time"

	"github.com/nearai/ironclaw-go/internal/db"
)

func TestStoreThreadHistory(t *testing.T) {
	database := db.NewMemoryDB()
	store := NewStore(database)
	ctx := context.Background()

	// 保存消息
	for i := 0; i < 5; i++ {
		_ = database.SaveMessage(ctx, &db.Message{
			ID:        "msg-" + string(rune('0'+i)),
			ThreadID:  "thread-1",
			UserID:    "user-1",
			Role:      "user",
			Content:   "hello",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	msgs, err := store.ThreadHistory(ctx, "thread-1", 10, 0)
	if err != nil {
		t.Fatalf("thread history: %v", err)
	}
	if len(msgs) != 5 {
		t.Errorf("msgs = %d, want 5", len(msgs))
	}
}

func TestStoreSearchMessages(t *testing.T) {
	database := db.NewMemoryDB()
	store := NewStore(database)
	ctx := context.Background()

	_ = database.SaveConversation(ctx, &db.Conversation{ID: "t1", UserID: "u1"})
	_ = database.SaveMessage(ctx, &db.Message{ID: "m1", ThreadID: "t1", UserID: "u1", Role: "user", Content: "how is the weather today"})
	_ = database.SaveMessage(ctx, &db.Message{ID: "m2", ThreadID: "t1", UserID: "u1", Role: "assistant", Content: "it is sunny"})

	results, err := store.SearchMessages(ctx, "u1", "weather", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !contains(results[0].Content, "weather") {
		t.Errorf("expected content to contain 'weather', got %q", results[0].Content)
	}
}

func TestStoreDeleteThread(t *testing.T) {
	database := db.NewMemoryDB()
	store := NewStore(database)
	ctx := context.Background()

	_ = database.SaveConversation(ctx, &db.Conversation{ID: "t1", UserID: "u1"})
	_ = database.SaveMessage(ctx, &db.Message{ID: "m1", ThreadID: "t1", UserID: "u1", Role: "user", Content: "hi"})

	if err := store.DeleteThread(ctx, "t1"); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	_, err := database.GetConversation(ctx, "t1")
	if err == nil {
		t.Error("expected conversation deleted")
	}
}

func TestStoreJobHistory(t *testing.T) {
	database := db.NewMemoryDB()
	store := NewStore(database)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_ = database.SaveJob(ctx, &db.Job{
			ID:     "job-" + string(rune('0'+i)),
			UserID: "u1",
			Name:   "test-job",
			Status: "completed",
		})
	}

	jobs, err := store.JobHistory(ctx, "u1", 10, 0)
	if err != nil {
		t.Fatalf("job history: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("jobs = %d, want 3", len(jobs))
	}
}

func TestAnalyticsJobStats(t *testing.T) {
	database := db.NewMemoryDB()
	store := NewStore(database)
	ctx := context.Background()

	_ = database.SaveJob(ctx, &db.Job{ID: "j1", UserID: "u1", Status: "completed", CreatedAt: time.Now(), UpdatedAt: time.Now().Add(time.Minute)})
	_ = database.SaveJob(ctx, &db.Job{ID: "j2", UserID: "u1", Status: "completed", CreatedAt: time.Now(), UpdatedAt: time.Now().Add(2 * time.Minute)})
	_ = database.SaveJob(ctx, &db.Job{ID: "j3", UserID: "u1", Status: "failed", CreatedAt: time.Now(), UpdatedAt: time.Now().Add(30 * time.Second)})

	stats, err := store.GetJobStats(ctx)
	if err != nil {
		t.Fatalf("job stats: %v", err)
	}

	if stats.TotalJobs != 3 {
		t.Errorf("total = %d, want 3", stats.TotalJobs)
	}
	if stats.CompletedJobs != 2 {
		t.Errorf("completed = %d, want 2", stats.CompletedJobs)
	}
	if stats.FailedJobs != 1 {
		t.Errorf("failed = %d, want 1", stats.FailedJobs)
	}
	if stats.SuccessRate < 0.66 || stats.SuccessRate > 0.67 {
		t.Errorf("success rate = %f, want ~0.666", stats.SuccessRate)
	}
	if stats.AvgDurationSecs <= 0 {
		t.Error("expected positive avg duration")
	}
}

func TestAnalyticsToolStats(t *testing.T) {
	database := db.NewMemoryDB()
	store := NewStore(database)
	ctx := context.Background()

	_ = database.SaveJob(ctx, &db.Job{ID: "j1", UserID: "u1", Status: "completed"})
	_ = database.SaveActionRecord(ctx, &db.ActionRecord{JobID: "j1", ToolName: "echo", Duration: 100 * time.Millisecond})
	_ = database.SaveActionRecord(ctx, &db.ActionRecord{JobID: "j1", ToolName: "echo", Duration: 200 * time.Millisecond})
	_ = database.SaveActionRecord(ctx, &db.ActionRecord{JobID: "j1", ToolName: "shell", Error: "oops", Duration: 50 * time.Millisecond})

	stats, err := store.GetToolStats(ctx)
	if err != nil {
		t.Fatalf("tool stats: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("stats len = %d, want 2", len(stats))
	}

	for _, s := range stats {
		if s.ToolName == "echo" {
			if s.TotalCalls != 2 {
				t.Errorf("echo total = %d, want 2", s.TotalCalls)
			}
			if s.SuccessfulCalls != 2 {
				t.Errorf("echo success = %d, want 2", s.SuccessfulCalls)
			}
			if s.AvgDurationMs != 150 {
				t.Errorf("echo avg ms = %f, want 150", s.AvgDurationMs)
			}
		}
		if s.ToolName == "shell" {
			if s.FailedCalls != 1 {
				t.Errorf("shell failed = %d, want 1", s.FailedCalls)
			}
			if s.SuccessRate != 0 {
				t.Errorf("shell success rate = %f, want 0", s.SuccessRate)
			}
		}
	}
}

func TestAnalyticsUserActivitySummary(t *testing.T) {
	database := db.NewMemoryDB()
	store := NewStore(database)
	ctx := context.Background()

	_ = database.SaveConversation(ctx, &db.Conversation{ID: "t1", UserID: "u1"})
	_ = database.SaveMessage(ctx, &db.Message{ID: "m1", ThreadID: "t1", UserID: "u1", Role: "user", Content: "hi", CreatedAt: time.Now()})

	_ = database.SaveJob(ctx, &db.Job{ID: "j1", UserID: "u1", Status: "completed", CreatedAt: time.Now()})
	_ = database.SaveActionRecord(ctx, &db.ActionRecord{JobID: "j1", ToolName: "echo", Duration: 100 * time.Millisecond})

	summary, err := store.GetUserActivitySummary(ctx, "u1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("activity summary: %v", err)
	}

	if summary.TotalMessages != 1 {
		t.Errorf("messages = %d, want 1", summary.TotalMessages)
	}
	if summary.TotalJobs != 1 {
		t.Errorf("jobs = %d, want 1", summary.TotalJobs)
	}
	if summary.UniqueToolsUsed != 1 {
		t.Errorf("unique tools = %d, want 1", summary.UniqueToolsUsed)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
