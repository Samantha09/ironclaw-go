package history

import (
	"context"
	"fmt"
	"time"
)

// JobStats 是作业执行统计。
type JobStats struct {
	TotalJobs       int
	CompletedJobs   int
	FailedJobs      int
	SuccessRate     float64
	AvgDurationSecs float64
}

// ToolStats 是单个工具的使用统计。
type ToolStats struct {
	ToolName       string
	TotalCalls     int
	SuccessfulCalls int
	FailedCalls    int
	SuccessRate    float64
	AvgDurationMs  float64
}

// ActivitySummary 是用户在时间段内的活动摘要。
type ActivitySummary struct {
	Period          string
	TotalMessages   int
	TotalJobs       int
	CompletedJobs   int
	FailedJobs      int
	UniqueToolsUsed int
	TopTools        []ToolStats
}

// GetJobStats 计算所有作业统计。
func (s *Store) GetJobStats(ctx context.Context) (JobStats, error) {
	jobs, err := s.db.ListJobs(ctx, "", 10000, 0)
	if err != nil {
		return JobStats{}, fmt.Errorf("list jobs: %w", err)
	}

	var total, completed, failed int
	var totalDuration time.Duration

	for _, j := range jobs {
		total++
		switch j.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		}
		d := j.UpdatedAt.Sub(j.CreatedAt)
		if d > 0 {
			totalDuration += d
		}
	}

	var avgSecs float64
	if total > 0 {
		avgSecs = totalDuration.Seconds() / float64(total)
	}

	successRate := 0.0
	if total > 0 {
		successRate = float64(completed) / float64(total)
	}

	return JobStats{
		TotalJobs:       total,
		CompletedJobs:   completed,
		FailedJobs:      failed,
		SuccessRate:     successRate,
		AvgDurationSecs: avgSecs,
	}, nil
}

// GetToolStats 计算工具使用统计。
func (s *Store) GetToolStats(ctx context.Context) ([]ToolStats, error) {
	jobs, err := s.db.ListJobs(ctx, "", 10000, 0)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}

	// 聚合每个工具的数据
	type agg struct {
		total      int
		success    int
		failed     int
		totalDurMs float64
	}
	m := make(map[string]*agg)

	for _, j := range jobs {
		recs, err := s.db.ListActionRecordsByJob(ctx, j.ID)
		if err != nil {
			continue
		}
		for _, rec := range recs {
			if _, ok := m[rec.ToolName]; !ok {
				m[rec.ToolName] = &agg{}
			}
			a := m[rec.ToolName]
			a.total++
			if rec.Error == "" {
				a.success++
			} else {
				a.failed++
			}
			a.totalDurMs += float64(rec.Duration.Milliseconds())
		}
	}

	var stats []ToolStats
	for name, a := range m {
		successRate := 0.0
		if a.total > 0 {
			successRate = float64(a.success) / float64(a.total)
		}
		avgMs := 0.0
		if a.total > 0 {
			avgMs = a.totalDurMs / float64(a.total)
		}
		stats = append(stats, ToolStats{
			ToolName:        name,
			TotalCalls:      a.total,
			SuccessfulCalls: a.success,
			FailedCalls:     a.failed,
			SuccessRate:     successRate,
			AvgDurationMs:   avgMs,
		})
	}

	return stats, nil
}

// GetUserActivitySummary 返回指定用户在时间段内的活动摘要。
func (s *Store) GetUserActivitySummary(ctx context.Context, userID string, since time.Time) (ActivitySummary, error) {
	convs, err := s.db.ListConversations(ctx, userID, 1000, 0)
	if err != nil {
		return ActivitySummary{}, fmt.Errorf("list conversations: %w", err)
	}

	var totalMessages int
	uniqueTools := make(map[string]struct{})

	for _, conv := range convs {
		msgs, err := s.db.GetMessagesByThread(ctx, conv.ID, 1000, 0)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			if msg.CreatedAt.After(since) || msg.CreatedAt.Equal(since) {
				totalMessages++
			}
		}
	}

	jobs, err := s.db.ListJobs(ctx, userID, 1000, 0)
	if err != nil {
		return ActivitySummary{}, fmt.Errorf("list jobs: %w", err)
	}

	var totalJobs, completedJobs, failedJobs int
	for _, j := range jobs {
		if j.CreatedAt.Before(since) {
			continue
		}
		totalJobs++
		switch j.Status {
		case "completed":
			completedJobs++
		case "failed":
			failedJobs++
		}

		recs, err := s.db.ListActionRecordsByJob(ctx, j.ID)
		if err != nil {
			continue
		}
		for _, rec := range recs {
			uniqueTools[rec.ToolName] = struct{}{}
		}
	}

	topTools, err := s.GetToolStats(ctx)
	if err != nil {
		topTools = nil
	}

	return ActivitySummary{
		Period:          since.Format(time.RFC3339),
		TotalMessages:   totalMessages,
		TotalJobs:       totalJobs,
		CompletedJobs:   completedJobs,
		FailedJobs:      failedJobs,
		UniqueToolsUsed: len(uniqueTools),
		TopTools:        topTools,
	}, nil
}
