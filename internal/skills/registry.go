package skills

import (
	"fmt"
	"strings"
	"sync"
)

// Registry 是线程安全的技能注册表。
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewRegistry 创建新的技能注册表。
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Skill)}
}

// Register 注册技能。若同名已存在则覆盖。
func (r *Registry) Register(s *Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[s.Name()] = s
}

// Get 按名称获取技能。
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// List 返回所有已注册技能名称。
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}

// All 返回所有已注册技能副本。
func (r *Registry) All() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	return out
}

// Count 返回注册技能数量。
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}

// Remove 移除指定技能。
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.skills, name)
}

// MatchSkills 返回对给定输入匹配的所有技能。
func (r *Registry) MatchSkills(input string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*Skill
	for _, s := range r.skills {
		if s.MatchInput(input) {
			matched = append(matched, s)
		}
	}
	return matched
}

// MinTrust 返回所有已注册技能中的最低信任级别。
// 若无技能，返回 TrustTrusted。
func (r *Registry) MinTrust() TrustLevel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.skills) == 0 {
		return TrustTrusted
	}

	min := TrustTrusted
	for _, s := range r.skills {
		if s.Trust < min {
			min = s.Trust
		}
	}
	return min
}

// BuildSystemPrompt 将所有技能的 prompt content 合并为系统提示。
func (r *Registry) BuildSystemPrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	for _, s := range r.skills {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString(fmt.Sprintf("# Skill: %s\n\n", s.Name()))
		sb.WriteString(s.Content)
	}
	return sb.String()
}
