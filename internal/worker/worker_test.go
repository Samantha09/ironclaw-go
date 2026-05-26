package worker

import (
	"context"
	"testing"
	"time"

	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/safety"
	"github.com/nearai/ironclaw-go/internal/tools"
	"github.com/nearai/ironclaw-go/internal/tools/builtin"
)

func TestPool(t *testing.T) {
	ctx := context.Background()
	database := db.NewMemoryDB()

	registry := tools.NewRegistry()
	registry.Register(builtin.NewEchoTool())

	dispatcher := tools.NewDispatcher(registry, safety.NewLayer(), database)

	t.Run("submit_job", func(t *testing.T) {
		pool := NewPool(database, dispatcher, 2)
		job, err := pool.Submit(ctx, "user1", "echo", `{"message":"hello"}`)
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
		if job.Status != "pending" {
			t.Errorf("expected pending, got %q", job.Status)
		}

		got, err := database.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetJob failed: %v", err)
		}
		if got.Name != "echo" {
			t.Errorf("expected echo, got %q", got.Name)
		}
	})

	t.Run("run_job", func(t *testing.T) {
		pool := NewPool(database, dispatcher, 2)
		pool.SetInterval(200 * time.Millisecond)
		pool.Start(ctx)
		defer pool.Stop()

		job, err := pool.Submit(ctx, "user1", "echo", `{"message":"async"}`)
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}

		var got *db.Job
		for i := 0; i < 50; i++ {
			got, err = database.GetJob(ctx, job.ID)
			if err != nil {
				t.Fatalf("GetJob failed: %v", err)
			}
			if got.Status == "completed" || got.Status == "failed" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if got.Status != "completed" {
			t.Fatalf("expected completed, got %q (error: %s)", got.Status, got.Error)
		}
		if got.Output != "async" {
			t.Errorf("expected 'async', got %q", got.Output)
		}
	})

	t.Run("run_invalid_tool", func(t *testing.T) {
		pool := NewPool(database, dispatcher, 2)
		pool.SetInterval(200 * time.Millisecond)
		pool.Start(ctx)
		defer pool.Stop()

		job, err := pool.Submit(ctx, "user1", "nonexistent", `{}`)
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}

		var got *db.Job
		for i := 0; i < 50; i++ {
			got, err = database.GetJob(ctx, job.ID)
			if err != nil {
				t.Fatalf("GetJob failed: %v", err)
			}
			if got.Status == "completed" || got.Status == "failed" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if got.Status != "failed" {
			t.Fatalf("expected failed, got %q", got.Status)
		}
	})
}
