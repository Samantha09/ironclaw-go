package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// JobManager 是作业存储和生命周期管理接口。
type JobManager interface {
	Create(ctx context.Context, userID, title, description string) (*Job, error)
	Get(ctx context.Context, jobID string) (*Job, bool)
	ListByUser(ctx context.Context, userID string) ([]*Job, error)
	UpdateStatus(ctx context.Context, jobID string, status JobStatus) error
	All(ctx context.Context) ([]*Job, error)
}

// MemoryJobManager 是内存中的 JobManager 实现。
type MemoryJobManager struct {
	mu       sync.RWMutex
	jobs     map[string]*Job
	byUser   map[string][]string // userID -> jobIDs
	maxJobs  int
}

// NewMemoryJobManager 创建新的内存作业管理器。
func NewMemoryJobManager(maxJobs int) *MemoryJobManager {
	return &MemoryJobManager{
		jobs:    make(map[string]*Job),
		byUser:  make(map[string][]string),
		maxJobs: maxJobs,
	}
}

// Create 创建新作业。
func (m *MemoryJobManager) Create(_ context.Context, userID, title, description string) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.maxJobs > 0 && len(m.jobs) >= m.maxJobs {
		return nil, fmt.Errorf("max concurrent jobs (%d) reached", m.maxJobs)
	}

	job := &Job{
		ID:          newJobID(),
		UserID:      userID,
		Title:       title,
		Description: description,
		Status:      JobPending,
		CreatedAt:   time.Now(),
	}
	m.jobs[job.ID] = job
	m.byUser[userID] = append(m.byUser[userID], job.ID)
	return job, nil
}

// Get 按 ID 获取作业。
func (m *MemoryJobManager) Get(_ context.Context, jobID string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[jobID]
	return j, ok
}

// ListByUser 列出用户的所有作业。
func (m *MemoryJobManager) ListByUser(_ context.Context, userID string) ([]*Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := m.byUser[userID]
	out := make([]*Job, 0, len(ids))
	for _, id := range ids {
		if j, ok := m.jobs[id]; ok {
			out = append(out, j)
		}
	}
	return out, nil
}

// UpdateStatus 更新作业状态。
func (m *MemoryJobManager) UpdateStatus(_ context.Context, jobID string, status JobStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %q not found", jobID)
	}

	now := time.Now()
	switch status {
	case JobInProgress:
		j.StartedAt = &now
	case JobCompleted, JobFailed, JobCancelled:
		j.CompletedAt = &now
	}
	j.Status = status
	return nil
}

// All 返回所有作业。
func (m *MemoryJobManager) All(_ context.Context) ([]*Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j)
	}
	return out, nil
}
