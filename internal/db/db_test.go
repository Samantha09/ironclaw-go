package db

import (
	"context"
	"testing"
	"time"
)

func TestMemoryDB(t *testing.T) {
	ctx := context.Background()
	db := NewMemoryDB()

	// Settings
	t.Run("settings", func(t *testing.T) {
		if err := db.SetSetting(ctx, "user1", "key1", "value1"); err != nil {
			t.Fatalf("SetSetting failed: %v", err)
		}
		val, err := db.GetSetting(ctx, "user1", "key1")
		if err != nil {
			t.Fatalf("GetSetting failed: %v", err)
		}
		if val != "value1" {
			t.Errorf("expected value1, got %q", val)
		}
		if err := db.DeleteSetting(ctx, "user1", "key1"); err != nil {
			t.Fatalf("DeleteSetting failed: %v", err)
		}
		_, err = db.GetSetting(ctx, "user1", "key1")
		if err == nil {
			t.Error("expected error after delete")
		}
	})

	// Conversations
	t.Run("conversations", func(t *testing.T) {
		conv := &Conversation{
			ID:      "conv1",
			UserID:  "user1",
			Channel: "repl",
			Title:   "Test",
		}
		if err := db.SaveConversation(ctx, conv); err != nil {
			t.Fatalf("SaveConversation failed: %v", err)
		}
		got, err := db.GetConversation(ctx, "conv1")
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
		if got.ID != "conv1" {
			t.Errorf("expected conv1, got %q", got.ID)
		}
	})

	// Messages
	t.Run("messages", func(t *testing.T) {
		msg := &Message{
			ID:       "msg1",
			ThreadID: "thread1",
			UserID:   "user1",
			Role:     "user",
			Content:  "hello",
		}
		if err := db.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}
		msgs, err := db.GetMessagesByThread(ctx, "thread1", 10, 0)
		if err != nil {
			t.Fatalf("GetMessagesByThread failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Errorf("expected 1 message, got %d", len(msgs))
		}
	})

	// Jobs
	t.Run("jobs", func(t *testing.T) {
		job := &Job{
			ID:     "job1",
			UserID: "user1",
			Name:   "test_job",
			Status: "pending",
		}
		if err := db.SaveJob(ctx, job); err != nil {
			t.Fatalf("SaveJob failed: %v", err)
		}
		if err := db.UpdateJobStatus(ctx, "job1", "completed", "done", ""); err != nil {
			t.Fatalf("UpdateJobStatus failed: %v", err)
		}
		got, err := db.GetJob(ctx, "job1")
		if err != nil {
			t.Fatalf("GetJob failed: %v", err)
		}
		if got.Status != "completed" {
			t.Errorf("expected completed, got %q", got.Status)
		}
	})

	// Action Records
	t.Run("action_records", func(t *testing.T) {
		rec := &ActionRecord{
			ID:       "rec1",
			JobID:    "job1",
			ToolName: "echo",
			Input:    `{"message":"hi"}`,
			Output:   "hi",
			Duration: time.Second,
		}
		if err := db.SaveActionRecord(ctx, rec); err != nil {
			t.Fatalf("SaveActionRecord failed: %v", err)
		}
		recs, err := db.ListActionRecordsByJob(ctx, "job1")
		if err != nil {
			t.Fatalf("ListActionRecordsByJob failed: %v", err)
		}
		if len(recs) != 1 {
			t.Errorf("expected 1 record, got %d", len(recs))
		}
	})
}
