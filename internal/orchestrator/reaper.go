package orchestrator

import (
	"context"
	"time"
)

// Reaper 清理过期或僵死的作业。
type Reaper interface {
	Cleanup(ctx context.Context, jm JobManager)
}

// MemoryReaper 是基于超时的内存清理器。
type MemoryReaper struct {
	jobTimeout time.Duration
}

// NewMemoryReaper 创建新的内存清理器。
func NewMemoryReaper(jobTimeout time.Duration) *MemoryReaper {
	return &MemoryReaper{jobTimeout: jobTimeout}
}

// Cleanup 扫描并标记超时的进行中的作业为失败。
func (r *MemoryReaper) Cleanup(ctx context.Context, jm JobManager) {
	jobs, err := jm.All(ctx)
	if err != nil {
		return
	}

	now := time.Now()
	for _, job := range jobs {
		if job.Status != JobInProgress {
			continue
		}
		if job.StartedAt == nil {
			continue
		}
		if now.Sub(*job.StartedAt) > r.jobTimeout {
			_ = jm.UpdateStatus(ctx, job.ID, JobFailed)
			job.Error = "timeout: job exceeded maximum duration"
		}
	}
}
