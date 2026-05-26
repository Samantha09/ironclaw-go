// Package worker 提供后台任务队列与工作池。
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/tools"
)

// Pool 管理后台任务的轮询与执行。
type Pool struct {
	db         db.Database
	dispatcher *tools.Dispatcher
	maxWorkers int
	interval   time.Duration
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewPool 创建新的后台任务池。
func NewPool(database db.Database, dispatcher *tools.Dispatcher, maxWorkers int) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	return &Pool{
		db:         database,
		dispatcher: dispatcher,
		maxWorkers: maxWorkers,
		interval:   5 * time.Second,
		stopCh:     make(chan struct{}),
	}
}

// SetInterval 设置轮询间隔（用于测试）。
func (p *Pool) SetInterval(d time.Duration) {
	p.interval = d
}

// Start 启动后台轮询循环。
func (p *Pool) Start(ctx context.Context) {
	p.wg.Add(1)
	go p.loop(ctx)
}

// Stop 停止后台轮询并等待当前任务完成。
func (p *Pool) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

// loop 常驻轮询循环。
func (p *Pool) loop(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	sem := make(chan struct{}, p.maxWorkers)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.pollAndRun(ctx, sem)
		}
	}
}

// pollAndRun 获取 pending 任务并分发给 worker goroutine。
func (p *Pool) pollAndRun(ctx context.Context, sem chan struct{}) {
	// 获取所有 pending 的任务（简化：不分用户，全局队列）
	jobs, err := p.db.ListJobs(ctx, "", 100, 0)
	if err != nil {
		return
	}

	for _, job := range jobs {
		if job.Status != "pending" {
			continue
		}

		select {
		case sem <- struct{}{}:
			p.wg.Add(1)
			go func(j *db.Job) {
				defer func() { <-sem }()
				defer p.wg.Done()
				p.runJob(context.Background(), j)
			}(job)
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// runJob 执行单个任务。
func (p *Pool) runJob(ctx context.Context, job *db.Job) {
	// 更新为 running
	if err := p.db.UpdateJobStatus(ctx, job.ID, "running", "", ""); err != nil {
		return
	}

	// 解析输入为工具参数
	var params map[string]any
	if job.Input != "" {
		_ = json.Unmarshal([]byte(job.Input), &params)
	}
	if params == nil {
		params = map[string]any{}
	}

	out, err := p.dispatcher.Dispatch(ctx, job.Name, params, &tools.JobContext{
		UserID:   job.UserID,
		JobID:    job.ID,
	})

	if err != nil {
		_ = p.db.UpdateJobStatus(ctx, job.ID, "failed", "", err.Error())
		return
	}

	_ = p.db.UpdateJobStatus(ctx, job.ID, "completed", out.Content, "")
}

// Submit 提交新任务到队列。
func (p *Pool) Submit(ctx context.Context, userID, name, input string) (*db.Job, error) {
	job := &db.Job{
		ID:      generateID(),
		UserID:  userID,
		Name:    name,
		Status:  "pending",
		Input:   input,
		CreatedAt: time.Now(),
	}
	if err := p.db.SaveJob(ctx, job); err != nil {
		return nil, fmt.Errorf("save job: %w", err)
	}
	return job, nil
}

var idCounter int
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("job_%d_%d", time.Now().Unix(), idCounter)
}
