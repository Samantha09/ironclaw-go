package skills

import (
	"fmt"
	"slices"

	"github.com/nearai/ironclaw-go/internal/llm"
)

// ReadOnlyTools 是无副作用的只读工具列表。
// 新增工具默认*不*在此列表中；需安全审查后方可加入。
var ReadOnlyTools = []string{
	"echo",
	"time",
	"json",
	"memory_read",
	"memory_search",
	"skill_list",
	"skill_search",
}

// AttenuationResult 是工具过滤结果。
type AttenuationResult struct {
	Tools       []llm.ToolDefinition
	MinTrust    TrustLevel
	Explanation string
	Removed     []string
}

// AttenuateTools 基于活跃技能的最低信任级别过滤工具。
// 这是硬安全边界：LLM 不知道被移除的工具，因此无法被操控调用它们。
func AttenuateTools(tools []llm.ToolDefinition, activeSkills []*Skill) AttenuationResult {
	if len(activeSkills) == 0 {
		return AttenuationResult{
			Tools:       tools,
			MinTrust:    TrustTrusted,
			Explanation: "No skills active, all tools available",
			Removed:     nil,
		}
	}

	minTrust := TrustTrusted
	for _, s := range activeSkills {
		if s.Trust < minTrust {
			minTrust = s.Trust
		}
	}

	switch minTrust {
	case TrustTrusted:
		return AttenuationResult{
			Tools:       tools,
			MinTrust:    minTrust,
			Explanation: "All active skills are trusted (full trust), all tools available",
			Removed:     nil,
		}
	case TrustInstalled:
		var kept []llm.ToolDefinition
		var removed []string

		for _, t := range tools {
			if slices.Contains(ReadOnlyTools, t.Function.Name) {
				kept = append(kept, t)
			} else {
				removed = append(removed, t.Function.Name)
			}
		}

		return AttenuationResult{
			Tools:       kept,
			MinTrust:    minTrust,
			Explanation: fmt.Sprintf("Installed skill present: restricted to read-only tools, removed %d tool(s): %s", len(removed), removed),
			Removed:     removed,
		}
	default:
		// 未知信任级别：保守策略，仅保留只读工具
		var kept []llm.ToolDefinition
		var removed []string
		for _, t := range tools {
			if slices.Contains(ReadOnlyTools, t.Function.Name) {
				kept = append(kept, t)
			} else {
				removed = append(removed, t.Function.Name)
			}
		}
		return AttenuationResult{
			Tools:       kept,
			MinTrust:    minTrust,
			Explanation: fmt.Sprintf("Unknown trust level: restricted to read-only tools, removed %d tool(s): %s", len(removed), removed),
			Removed:     removed,
		}
	}
}
