// Package history 提供对话历史查询、审计日志聚合与分析统计。
package history

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nearai/ironclaw-go/internal/db"
)

// Store 是 history 的高级操作层，包装 db.Database。
type Store struct {
	db db.Database
}

// NewStore 创建新的 history store。
func NewStore(database db.Database) *Store {
	return &Store{db: database}
}

// ThreadHistory 返回指定线程的消息历史。
func (s *Store) ThreadHistory(ctx context.Context, threadID string, limit, offset int) ([]*db.Message, error) {
	msgs, err := s.db.GetMessagesByThread(ctx, threadID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}
	return msgs, nil
}

// UserConversations 返回用户的所有对话线程摘要。
func (s *Store) UserConversations(ctx context.Context, userID string, limit, offset int) ([]*db.Conversation, error) {
	convs, err := s.db.ListConversations(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return convs, nil
}

// RecentMessages 返回用户最近的消息（跨所有线程）。
// 通过遍历用户对话实现；生产环境可添加专用索引。
func (s *Store) RecentMessages(ctx context.Context, userID string, limit int) ([]*db.Message, error) {
	convs, err := s.db.ListConversations(ctx, userID, 100, 0)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}

	var all []*db.Message
	for _, conv := range convs {
		msgs, err := s.db.GetMessagesByThread(ctx, conv.ID, limit, 0)
		if err != nil {
			continue
		}
		all = append(all, msgs...)
	}

	// 按时间倒序截断
	sortMessagesDesc(all)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// ArchiveThread 归档线程（删除消息但保留对话元数据）。
func (s *Store) ArchiveThread(ctx context.Context, threadID string) error {
	if err := s.db.DeleteMessagesByThread(ctx, threadID); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	return nil
}

// DeleteThread 完全删除线程及其消息。
func (s *Store) DeleteThread(ctx context.Context, threadID string) error {
	if err := s.db.DeleteMessagesByThread(ctx, threadID); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	if err := s.db.DeleteConversation(ctx, threadID); err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

// JobHistory 返回用户的后台作业历史。
func (s *Store) JobHistory(ctx context.Context, userID string, limit, offset int) ([]*db.Job, error) {
	jobs, err := s.db.ListJobs(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}

// ActionHistory 返回指定作业的工具调用审计记录。
func (s *Store) ActionHistory(ctx context.Context, jobID string) ([]*db.ActionRecord, error) {
	recs, err := s.db.ListActionRecordsByJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("list action records: %w", err)
	}
	return recs, nil
}

func sortMessagesDesc(msgs []*db.Message) {
	// 简单冒泡排序，数据量小
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].CreatedAt.After(msgs[i].CreatedAt) {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
}

// SearchResult 是历史搜索的匹配项。
type SearchResult struct {
	ThreadID    string
	MessageID   string
	Role        string
	Content     string
	MatchedText string
	CreatedAt   time.Time
}

// SearchMessages 在用户的所有消息中搜索包含 query 的消息。
func (s *Store) SearchMessages(ctx context.Context, userID, query string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}
	queryLower := strings.ToLower(query)

	convs, err := s.db.ListConversations(ctx, userID, 100, 0)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}

	var results []SearchResult
	for _, conv := range convs {
		msgs, err := s.db.GetMessagesByThread(ctx, conv.ID, 1000, 0)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			if strings.Contains(strings.ToLower(msg.Content), queryLower) {
				results = append(results, SearchResult{
					ThreadID:    msg.ThreadID,
					MessageID:   msg.ID,
					Role:        msg.Role,
					Content:     msg.Content,
					MatchedText: extractSnippet(msg.Content, query, 80),
					CreatedAt:   msg.CreatedAt,
				})
				if len(results) >= limit {
					return results, nil
				}
			}
		}
	}
	return results, nil
}

func extractSnippet(content, query string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	idx := strings.Index(strings.ToLower(content), strings.ToLower(query))
	if idx < 0 {
		if len(content) > maxLen {
			return content[:maxLen] + "..."
		}
		return content
	}
	start := idx - maxLen/4
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(content) {
		end = len(content)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}
	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}
	return snippet
}
