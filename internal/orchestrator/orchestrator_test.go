package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestJobStatusString(t *testing.T) {
	cases := []struct {
		status JobStatus
		want   string
	}{
		{JobPending, "pending"},
		{JobInProgress, "in_progress"},
		{JobCompleted, "completed"},
		{JobFailed, "failed"},
		{JobCancelled, "cancelled"},
		{JobStatus(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestJobStatusIsTerminal(t *testing.T) {
	terminal := []JobStatus{JobCompleted, JobFailed, JobCancelled}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%v should be terminal", s)
		}
	}
	nonTerminal := []JobStatus{JobPending, JobInProgress}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%v should not be terminal", s)
		}
	}
}

func TestOrchestratorCreateJob(t *testing.T) {
	o := New(DefaultConfig())
	ctx := context.Background()

	job, err := o.CreateJob(ctx, "user1", "test job", "desc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected job ID")
	}
	if job.Status != JobPending {
		t.Errorf("status = %v, want pending", job.Status)
	}
}

func TestOrchestratorJobToken(t *testing.T) {
	o := New(DefaultConfig())
	ctx := context.Background()

	job, _ := o.CreateJob(ctx, "user1", "test", "desc")
	tok, ok := o.JobToken(job.ID)
	if !ok {
		t.Fatal("expected token to be generated")
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}

	if !o.ValidateToken(job.ID, tok) {
		t.Error("expected token to be valid")
	}
	if o.ValidateToken(job.ID, "wrong") {
		t.Error("expected wrong token to be invalid")
	}
}

func TestOrchestratorListJobs(t *testing.T) {
	o := New(DefaultConfig())
	ctx := context.Background()

	o.CreateJob(ctx, "user1", "job1", "d1")
	o.CreateJob(ctx, "user1", "job2", "d2")
	o.CreateJob(ctx, "user2", "job3", "d3")

	jobs, err := o.ListJobs(ctx, "user1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("jobs = %d, want 2", len(jobs))
	}
}

func TestOrchestratorGetJob(t *testing.T) {
	ctx := context.Background()
	o := New(DefaultConfig())
	job, _ := o.CreateJob(ctx, "user1", "test", "desc")

	got, ok := o.GetJob(ctx, job.ID)
	if !ok {
		t.Fatal("expected to find job")
	}
	if got.ID != job.ID {
		t.Error("ID mismatch")
	}

	_, ok = o.GetJob(ctx, "missing")
	if ok {
		t.Error("expected not found")
	}
}

func TestOrchestratorCancelJob(t *testing.T) {
	ctx := context.Background()
	o := New(DefaultConfig())
	job, _ := o.CreateJob(ctx, "user1", "test", "desc")

	if err := o.CancelJob(ctx, job.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	got, _ := o.GetJob(ctx, job.ID)
	if got.Status != JobCancelled {
		t.Errorf("status = %v, want cancelled", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestMemoryJobManagerMaxJobs(t *testing.T) {
	jm := NewMemoryJobManager(1)
	ctx := context.Background()

	_, err := jm.Create(ctx, "u1", "j1", "d1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = jm.Create(ctx, "u1", "j2", "d2")
	if err == nil {
		t.Error("expected error when max jobs reached")
	}
}

func TestMemoryJobManagerUpdateStatus(t *testing.T) {
	jm := NewMemoryJobManager(10)
	ctx := context.Background()

	job, _ := jm.Create(ctx, "u1", "test", "desc")
	if err := jm.UpdateStatus(ctx, job.ID, JobInProgress); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := jm.Get(ctx, job.ID)
	if got.Status != JobInProgress {
		t.Errorf("status = %v", got.Status)
	}
	if got.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}

	if err := jm.UpdateStatus(ctx, "missing", JobCompleted); err == nil {
		t.Error("expected error for missing job")
	}
}

func TestTokenStore(t *testing.T) {
	ts := NewTokenStore()

	tok1 := ts.Generate("job1")
	tok2 := ts.Generate("job2")

	if tok1 == "" || tok2 == "" {
		t.Error("expected non-empty tokens")
	}
	if tok1 == tok2 {
		t.Error("expected different tokens")
	}

	if !ts.Validate("job1", tok1) {
		t.Error("expected tok1 to validate for job1")
	}
	if ts.Validate("job1", tok2) {
		t.Error("expected tok2 not to validate for job1")
	}

	got, ok := ts.Get("job1")
	if !ok || got != tok1 {
		t.Error("expected Get to return tok1")
	}

	ts.Revoke("job1")
	if _, ok := ts.Get("job1"); ok {
		t.Error("expected token to be revoked")
	}
}

func TestMemoryReaperCleanup(t *testing.T) {
	jm := NewMemoryJobManager(10)
	ctx := context.Background()

	job, _ := jm.Create(ctx, "u1", "old", "desc")
	jm.UpdateStatus(ctx, job.ID, JobInProgress)
	// 手动将 StartedAt 设为很久以前
	oldTime := time.Now().Add(-2 * time.Hour)
	job.StartedAt = &oldTime

	rpr := NewMemoryReaper(1 * time.Hour)
	rpr.Cleanup(ctx, jm)

	got, _ := jm.Get(ctx, job.ID)
	if got.Status != JobFailed {
		t.Errorf("status = %v, want failed", got.Status)
	}
	if got.Error == "" {
		t.Error("expected error message after timeout")
	}
}

func TestMemoryReaperSkipsRecentJobs(t *testing.T) {
	jm := NewMemoryJobManager(10)
	ctx := context.Background()

	job, _ := jm.Create(ctx, "u1", "recent", "desc")
	jm.UpdateStatus(ctx, job.ID, JobInProgress)

	rpr := NewMemoryReaper(1 * time.Hour)
	rpr.Cleanup(ctx, jm)

	got, _ := jm.Get(ctx, job.ID)
	if got.Status != JobInProgress {
		t.Errorf("status = %v, want in_progress", got.Status)
	}
}

func TestMemoryReaperSkipsNonInProgress(t *testing.T) {
	jm := NewMemoryJobManager(10)
	ctx := context.Background()

	job, _ := jm.Create(ctx, "u1", "pending", "desc")
	// status is pending, not in_progress

	oldTime := time.Now().Add(-2 * time.Hour)
	job.StartedAt = &oldTime

	rpr := NewMemoryReaper(1 * time.Hour)
	rpr.Cleanup(ctx, jm)

	got, _ := jm.Get(ctx, job.ID)
	if got.Status != JobPending {
		t.Errorf("status = %v, want pending", got.Status)
	}
}

func TestOrchestratorStartReaper(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReaperInterval = 50 * time.Millisecond
	cfg.JobTimeout = 1 * time.Millisecond

	o := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job, _ := o.CreateJob(ctx, "u1", "test", "desc")
	o.jm.UpdateStatus(ctx, job.ID, JobInProgress)
	oldTime := time.Now().Add(-2 * time.Hour)
	job.StartedAt = &oldTime

	o.StartReaper(ctx)
	time.Sleep(100 * time.Millisecond)

	got, _ := o.GetJob(ctx, job.ID)
	if got.Status != JobFailed {
		t.Errorf("status = %v, want failed", got.Status)
	}
}
