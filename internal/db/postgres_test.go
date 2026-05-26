package db

import (
	"context"
	"os"
	"testing"
	"time"
)

// postgresDSN 返回测试用的 PostgreSQL 连接字符串。
// 如果环境变量 IRONCLAW_TEST_POSTGRES_DSN 未设置，测试将被跳过。
func postgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("IRONCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IRONCLAW_TEST_POSTGRES_DSN not set, skipping PostgreSQL tests")
	}
	return dsn
}

func TestPostgresDB(t *testing.T) {
	ctx := context.Background()
	dsn := postgresDSN(t)

	db, err := NewPostgresDB(dsn, 5, 1)
	if err != nil {
		t.Fatalf("NewPostgresDB failed: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	// 每个子测试使用不同的用户/ID 避免冲突
	userID := "test_user_" + time.Now().Format("20060102150405")

	t.Run("settings", func(t *testing.T) {
		if err := db.SetSetting(ctx, userID, "key1", "value1"); err != nil {
			t.Fatalf("SetSetting failed: %v", err)
		}
		val, err := db.GetSetting(ctx, userID, "key1")
		if err != nil {
			t.Fatalf("GetSetting failed: %v", err)
		}
		if val != "value1" {
			t.Errorf("expected value1, got %q", val)
		}
		if err := db.SetSetting(ctx, userID, "key1", "value2"); err != nil {
			t.Fatalf("SetSetting update failed: %v", err)
		}
		val, err = db.GetSetting(ctx, userID, "key1")
		if err != nil {
			t.Fatalf("GetSetting after update failed: %v", err)
		}
		if val != "value2" {
			t.Errorf("expected value2, got %q", val)
		}
		if err := db.DeleteSetting(ctx, userID, "key1"); err != nil {
			t.Fatalf("DeleteSetting failed: %v", err)
		}
		_, err = db.GetSetting(ctx, userID, "key1")
		if err == nil {
			t.Error("expected error after delete")
		}
	})

	t.Run("conversations", func(t *testing.T) {
		conv := &Conversation{
			ID:      userID + "_conv1",
			UserID:  userID,
			Channel: "repl",
			Title:   "Test Conversation",
		}
		if err := db.SaveConversation(ctx, conv); err != nil {
			t.Fatalf("SaveConversation failed: %v", err)
		}

		got, err := db.GetConversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
		if got.ID != conv.ID {
			t.Errorf("expected %s, got %q", conv.ID, got.ID)
		}
		if got.Title != conv.Title {
			t.Errorf("expected %s, got %q", conv.Title, got.Title)
		}

		list, err := db.ListConversations(ctx, userID, 10, 0)
		if err != nil {
			t.Fatalf("ListConversations failed: %v", err)
		}
		found := false
		for _, c := range list {
			if c.ID == conv.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("conversation %s not found in list", conv.ID)
		}

		// 更新
		conv.Title = "Updated"
		if err := db.SaveConversation(ctx, conv); err != nil {
			t.Fatalf("SaveConversation update failed: %v", err)
		}
		got, err = db.GetConversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("GetConversation after update failed: %v", err)
		}
		if got.Title != "Updated" {
			t.Errorf("expected Updated, got %q", got.Title)
		}
	})

	t.Run("messages", func(t *testing.T) {
		convID := userID + "_msg_conv"
		conv := &Conversation{
			ID:      convID,
			UserID:  userID,
			Channel: "repl",
			Title:   "Message Test",
		}
		if err := db.SaveConversation(ctx, conv); err != nil {
			t.Fatalf("SaveConversation failed: %v", err)
		}

		msg := &Message{
			ID:       userID + "_msg1",
			ThreadID: convID,
			UserID:   userID,
			Role:     "user",
			Content:  "hello postgres",
			ToolCalls: []ToolCall{
				{ID: "tc1", Name: "echo", Arguments: `{"msg":"hi"}`, Result: "hi", Status: "success"},
			},
		}
		if err := db.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}

		msgs, err := db.GetMessagesByThread(ctx, convID, 10, 0)
		if err != nil {
			t.Fatalf("GetMessagesByThread failed: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if msgs[0].Content != "hello postgres" {
			t.Errorf("expected 'hello postgres', got %q", msgs[0].Content)
		}
		if len(msgs[0].ToolCalls) != 1 {
			t.Errorf("expected 1 tool call, got %d", len(msgs[0].ToolCalls))
		}
		if msgs[0].ToolCalls[0].Name != "echo" {
			t.Errorf("expected tool call name echo, got %q", msgs[0].ToolCalls[0].Name)
		}
	})

	t.Run("jobs", func(t *testing.T) {
		job := &Job{
			ID:     userID + "_job1",
			UserID: userID,
			Name:   "test_job",
			Status: "pending",
		}
		if err := db.SaveJob(ctx, job); err != nil {
			t.Fatalf("SaveJob failed: %v", err)
		}

		if err := db.UpdateJobStatus(ctx, job.ID, "completed", "done", ""); err != nil {
			t.Fatalf("UpdateJobStatus failed: %v", err)
		}

		got, err := db.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetJob failed: %v", err)
		}
		if got.Status != "completed" {
			t.Errorf("expected completed, got %q", got.Status)
		}
		if got.Output != "done" {
			t.Errorf("expected output 'done', got %q", got.Output)
		}

		list, err := db.ListJobs(ctx, userID, 10, 0)
		if err != nil {
			t.Fatalf("ListJobs failed: %v", err)
		}
		found := false
		for _, j := range list {
			if j.ID == job.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("job %s not found in list", job.ID)
		}
	})

	t.Run("action_records", func(t *testing.T) {
		jobID := userID + "_action_job"
		job := &Job{
			ID:     jobID,
			UserID: userID,
			Name:   "action_test",
			Status: "pending",
		}
		if err := db.SaveJob(ctx, job); err != nil {
			t.Fatalf("SaveJob failed: %v", err)
		}

		rec := &ActionRecord{
			ID:       userID + "_rec1",
			JobID:    jobID,
			ToolName: "echo",
			Input:    `{"message":"hi"}`,
			Output:   "hi",
			Duration: 1500 * time.Millisecond,
		}
		if err := db.SaveActionRecord(ctx, rec); err != nil {
			t.Fatalf("SaveActionRecord failed: %v", err)
		}

		recs, err := db.ListActionRecordsByJob(ctx, jobID)
		if err != nil {
			t.Fatalf("ListActionRecordsByJob failed: %v", err)
		}
		if len(recs) != 1 {
			t.Fatalf("expected 1 record, got %d", len(recs))
		}
		if recs[0].ToolName != "echo" {
			t.Errorf("expected echo, got %q", recs[0].ToolName)
		}
		if recs[0].Duration != 1500*time.Millisecond {
			t.Errorf("expected duration 1500ms, got %v", recs[0].Duration)
		}
	})
}
