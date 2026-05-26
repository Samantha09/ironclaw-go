package skills

import (
	"embed"
	"testing"

	"github.com/nearai/ironclaw-go/internal/llm"
)

//go:embed testdata/*.md
var testFS embed.FS

func TestParseManifest(t *testing.T) {
	content := `+++
name = "github"
version = "1.0.0"
description = "GitHub helper"

[activation]
keywords = ["github", "pr", "issue"]
exclude_keywords = ["gitlab"]
tags = ["dev"]
patterns = ["^gh\\s"]

[[credentials]]
name = "github_token"
provider = "github"
location = { type = "bearer" }
hosts = ["api.github.com"]
+++

You are a GitHub assistant. Help the user with PRs and issues.
`

	m, body, err := ParseManifest(content)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	if m.Name != "github" {
		t.Errorf("name = %q, want github", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", m.Version)
	}
	if len(m.Activation.Keywords) != 3 {
		t.Errorf("keywords len = %d, want 3", len(m.Activation.Keywords))
	}
	if len(m.Credentials) != 1 {
		t.Fatalf("credentials len = %d, want 1", len(m.Credentials))
	}
	if m.Credentials[0].Name != "github_token" {
		t.Errorf("credential name = %q, want github_token", m.Credentials[0].Name)
	}
	if body == "" {
		t.Error("body should not be empty")
	}
}

func TestParseManifest_NoFrontmatter(t *testing.T) {
	content := "Just plain markdown without frontmatter."
	m, body, err := ParseManifest(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Error("expected nil manifest when no frontmatter")
	}
	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func TestManifestValidate(t *testing.T) {
	cases := []struct {
		name    string
		m       Manifest
		wantErr bool
	}{
		{"ok", Manifest{Name: "x"}, false},
		{"missing name", Manifest{}, true},
		{"missing credential name", Manifest{Name: "x", Credentials: []CredentialSpec{{Provider: "p"}}}, true},
		{"missing credential provider", Manifest{Name: "x", Credentials: []CredentialSpec{{Name: "n"}}}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSkillMatchInput(t *testing.T) {
	s := &Skill{
		Manifest: Manifest{
			Activation: Activation{
				Keywords:        []string{"github", "pr"},
				ExcludeKeywords: []string{"gitlab"},
			},
		},
	}

	cases := []struct {
		input string
		want  bool
	}{
		{"create a github pr", true},
		{"how do i open a pr", true},
		{"gitlab pr", false},
		{"random chat", false},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := s.MatchInput(tc.input)
			if got != tc.want {
				t.Errorf("MatchInput(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestSkillMatchInput_NoKeywords(t *testing.T) {
	s := &Skill{Manifest: Manifest{Activation: Activation{}}}
	if !s.MatchInput("anything") {
		t.Error("expected true when no keywords defined")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	r.Register(&Skill{Manifest: Manifest{Name: "a"}, Trust: TrustTrusted})
	r.Register(&Skill{Manifest: Manifest{Name: "b"}, Trust: TrustInstalled})

	if r.Count() != 2 {
		t.Errorf("count = %d, want 2", r.Count())
	}

	s, ok := r.Get("a")
	if !ok || s.Name() != "a" {
		t.Error("expected to get skill a")
	}

	if _, ok := r.Get("missing"); ok {
		t.Error("expected not found")
	}

	names := r.List()
	if len(names) != 2 {
		t.Errorf("list len = %d, want 2", len(names))
	}

	all := r.All()
	if len(all) != 2 {
		t.Errorf("all len = %d, want 2", len(all))
	}

	r.Remove("a")
	if r.Count() != 1 {
		t.Errorf("count after remove = %d, want 1", r.Count())
	}
}

func TestRegistryMatchSkills(t *testing.T) {
	r := NewRegistry()
	r.Register(&Skill{Manifest: Manifest{Name: "gh", Activation: Activation{Keywords: []string{"github"}}}})
	r.Register(&Skill{Manifest: Manifest{Name: "time", Activation: Activation{Keywords: []string{"time"}}}})

	matched := r.MatchSkills("how to use github")
	if len(matched) != 1 || matched[0].Name() != "gh" {
		t.Errorf("matched = %v, want [gh]", matched)
	}
}

func TestRegistryMinTrust(t *testing.T) {
	r := NewRegistry()
	if r.MinTrust() != TrustTrusted {
		t.Errorf("empty registry min trust = %v, want Trusted", r.MinTrust())
	}

	r.Register(&Skill{Manifest: Manifest{Name: "a"}, Trust: TrustTrusted})
	if r.MinTrust() != TrustTrusted {
		t.Error("expected Trusted")
	}

	r.Register(&Skill{Manifest: Manifest{Name: "b"}, Trust: TrustInstalled})
	if r.MinTrust() != TrustInstalled {
		t.Error("expected Installed")
	}
}

func TestRegistryBuildSystemPrompt(t *testing.T) {
	r := NewRegistry()
	r.Register(&Skill{Manifest: Manifest{Name: "a"}, Content: "Do A."})
	r.Register(&Skill{Manifest: Manifest{Name: "b"}, Content: "Do B."})

	prompt := r.BuildSystemPrompt()
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !contains(prompt, "Skill: a") || !contains(prompt, "Skill: b") {
		t.Errorf("prompt missing skill headers: %s", prompt)
	}
}

func TestAttenuateTools_NoSkills(t *testing.T) {
	tools := []llm.ToolDefinition{
		{Function: llm.FunctionSchema{Name: "shell"}},
		{Function: llm.FunctionSchema{Name: "time"}},
	}

	res := AttenuateTools(tools, nil)
	if len(res.Tools) != 2 {
		t.Errorf("kept = %d, want 2", len(res.Tools))
	}
	if len(res.Removed) != 0 {
		t.Errorf("removed = %v, want empty", res.Removed)
	}
}

func TestAttenuateTools_Trusted(t *testing.T) {
	tools := []llm.ToolDefinition{
		{Function: llm.FunctionSchema{Name: "shell"}},
		{Function: llm.FunctionSchema{Name: "time"}},
	}

	active := []*Skill{{Trust: TrustTrusted}}
	res := AttenuateTools(tools, active)
	if len(res.Tools) != 2 {
		t.Errorf("kept = %d, want 2", len(res.Tools))
	}
	if res.MinTrust != TrustTrusted {
		t.Errorf("min trust = %v, want Trusted", res.MinTrust)
	}
}

func TestAttenuateTools_Installed(t *testing.T) {
	tools := []llm.ToolDefinition{
		{Function: llm.FunctionSchema{Name: "shell"}},
		{Function: llm.FunctionSchema{Name: "http"}},
		{Function: llm.FunctionSchema{Name: "time"}},
		{Function: llm.FunctionSchema{Name: "echo"}},
	}

	active := []*Skill{{Trust: TrustInstalled}}
	res := AttenuateTools(tools, active)

	if len(res.Tools) != 2 {
		t.Errorf("kept = %d, want 2", len(res.Tools))
	}
	if len(res.Removed) != 2 {
		t.Errorf("removed = %v, want 2", len(res.Removed))
	}

	keptNames := make([]string, len(res.Tools))
	for i, t := range res.Tools {
		keptNames[i] = t.Function.Name
	}
	if !slicesContains(keptNames, "time") || !slicesContains(keptNames, "echo") {
		t.Errorf("kept names = %v, want time+echo", keptNames)
	}
	if res.MinTrust != TrustInstalled {
		t.Errorf("min trust = %v, want Installed", res.MinTrust)
	}
}

func TestAttenuateTools_MixedTrust(t *testing.T) {
	tools := []llm.ToolDefinition{
		{Function: llm.FunctionSchema{Name: "shell"}},
		{Function: llm.FunctionSchema{Name: "time"}},
	}

	active := []*Skill{
		{Trust: TrustTrusted},
		{Trust: TrustInstalled},
	}
	res := AttenuateTools(tools, active)
	if res.MinTrust != TrustInstalled {
		t.Errorf("mixed min trust = %v, want Installed", res.MinTrust)
	}
	if len(res.Tools) != 1 || res.Tools[0].Function.Name != "time" {
		t.Errorf("kept = %v, want [time]", res.Tools)
	}
}

func TestLoadBundledSkills(t *testing.T) {
	skills, err := LoadBundledSkills(testFS, "testdata")
	if err != nil {
		t.Fatalf("load bundled: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 bundled skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Name() != "weather" {
		t.Errorf("name = %q, want weather", s.Name())
	}
	if s.Trust != TrustTrusted {
		t.Errorf("trust = %v, want Trusted", s.Trust)
	}
	if s.Source != SourceBundled {
		t.Errorf("source = %v, want Bundled", s.Source)
	}
	if s.Content == "" {
		t.Error("expected non-empty content")
	}
}

func contains(s, substr string) bool {
	return stringsContains(s, substr)
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func slicesContains[S ~[]E, E comparable](s S, v E) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}
