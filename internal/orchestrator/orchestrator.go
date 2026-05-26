// Package orchestrator 负责任业编排与高阶工作流调度。
// MVP 提供内存中的作业管理和 token 存储骨架，Docker 容器生命周期留待后续实现。
package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// JobStatus 表示作业状态。
type JobStatus int

const (
	JobPending JobStatus = iota
	JobInProgress
	JobCompleted
	JobFailed
	JobCancelled
)

func (s JobStatus) String() string {
	switch s {
	case JobPending:
		return "pending"
	case JobInProgress:
		return "in_progress"
	case JobCompleted:
		return "completed"
	case JobFailed:
		return "failed"
	case JobCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// IsTerminal 返回是否为终止状态。
func (s JobStatus) IsTerminal() bool {
	return s == JobCompleted || s == JobFailed || s == JobCancelled
}

// Job 表示一个编排作业。
type Job struct {
	ID          string
	UserID      string
	Title       string
	Description string
	Status      JobStatus
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       string
	Result      string
}

// Config 是 orchestrator 的行为配置。
type Config struct {
	MaxConcurrentJobs int
	JobTimeout        time.Duration
	ReaperInterval    time.Duration
}

// DefaultConfig 返回合理的默认配置。
func DefaultConfig() Config {
	return Config{
		MaxConcurrentJobs: 4,
		JobTimeout:        30 * time.Minute,
		ReaperInterval:    5 * time.Minute,
	}
}

// Orchestrator 是高层作业编排器。
type Orchestrator struct {
	config Config
	jm     JobManager
	ts     *TokenStore
	rpr    Reaper
}

// New 创建新的 orchestrator 实例。
func New(cfg Config) *Orchestrator {
	jm := NewMemoryJobManager(cfg.MaxConcurrentJobs)
	ts := NewTokenStore()
	rpr := NewMemoryReaper(cfg.JobTimeout)
	return &Orchestrator{
		config: cfg,
		jm:     jm,
		ts:     ts,
		rpr:    rpr,
	}
}

// CreateJob 为用户创建新作业。
func (o *Orchestrator) CreateJob(ctx context.Context, userID, title, description string) (*Job, error) {
	job, err := o.jm.Create(ctx, userID, title, description)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	// 生成作业 token
	o.ts.Generate(job.ID)
	return job, nil
}

// ListJobs 列出用户的所有作业。
func (o *Orchestrator) ListJobs(ctx context.Context, userID string) ([]*Job, error) {
	return o.jm.ListByUser(ctx, userID)
}

// GetJob 获取指定作业。
func (o *Orchestrator) GetJob(ctx context.Context, jobID string) (*Job, bool) {
	return o.jm.Get(ctx, jobID)
}

// CancelJob 取消作业。
func (o *Orchestrator) CancelJob(ctx context.Context, jobID string) error {
	if err := o.jm.UpdateStatus(ctx, jobID, JobCancelled); err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	return nil
}

// ValidateToken 验证作业 token。
func (o *Orchestrator) ValidateToken(jobID, token string) bool {
	return o.ts.Validate(jobID, token)
}

// JobToken 获取作业的 token。
func (o *Orchestrator) JobToken(jobID string) (string, bool) {
	return o.ts.Get(jobID)
}

// StartReaper 启动后台清理循环。
func (o *Orchestrator) StartReaper(ctx context.Context) {
	if o.rpr == nil {
		return
	}
	ticker := time.NewTicker(o.config.ReaperInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				o.rpr.Cleanup(ctx, o.jm)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// newJobID 生成新的作业 ID。
func newJobID() string {
	return uuid.New().String()
}
