package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nearai/ironclaw-go/internal/extensions"
)

func loadManifestFromString(s string) (*ExtensionManifest, error) {
	var m ExtensionManifest
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func loadBundlesFromString(s string) (*BundlesFile, error) {
	var b BundlesFile
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		return nil, err
	}
	if b.Bundles == nil {
		b.Bundles = make(map[string]BundleDefinition)
	}
	return &b, nil
}

func TestLoadManifest(t *testing.T) {
	json := `{
		"name": "slack",
		"display_name": "Slack",
		"kind": "tool",
		"version": "0.1.0",
		"description": "Post messages via Slack API",
		"keywords": ["messaging"],
		"source": {
			"dir": "tools-src/slack",
			"capabilities": "slack-tool.capabilities.json",
			"crate_name": "slack-tool"
		},
		"artifacts": {
			"wasm32-wasip2": { "url": null, "sha256": null }
		},
		"auth_summary": {
			"method": "oauth",
			"provider": "Slack",
			"secrets": ["slack_bot_token"]
		},
		"tags": ["default", "messaging"]
	}`

	m, err := loadManifestFromString(json)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if m.Name != "slack" {
		t.Errorf("name = %q, want slack", m.Name)
	}
	if m.Kind != KindTool {
		t.Errorf("kind = %q, want tool", m.Kind)
	}
	if m.Version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", m.Version)
	}
	if len(m.Tags) != 2 {
		t.Errorf("tags = %d, want 2", len(m.Tags))
	}
	if m.Hidden {
		t.Error("expected hidden=false by default")
	}

	entry, err := m.ToRegistryEntry()
	if err != nil {
		t.Fatalf("to registry entry: %v", err)
	}
	if entry.Kind != extensions.KindWasmTool {
		t.Errorf("entry kind = %v, want WasmTool", entry.Kind)
	}
	if entry.Hidden {
		t.Error("expected entry hidden=false")
	}
}

func TestLoadManifestHidden(t *testing.T) {
	json := `{
		"name": "telegram_mtproto",
		"display_name": "Telegram Tool",
		"kind": "tool",
		"version": "0.2.1",
		"hidden": true,
		"description": "Direct MTProto integration",
		"source": {
			"dir": "tools-src/telegram",
			"capabilities": "telegram-tool.capabilities.json",
			"crate_name": "telegram-tool"
		}
	}`

	m, err := loadManifestFromString(json)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if !m.Hidden {
		t.Error("expected hidden=true")
	}

	entry, err := m.ToRegistryEntry()
	if err != nil {
		t.Fatalf("to registry entry: %v", err)
	}
	if !entry.Hidden {
		t.Error("expected entry hidden=true")
	}
}

func TestLoadMcpManifest(t *testing.T) {
	json := `{
		"name": "notion",
		"display_name": "Notion",
		"kind": "mcp_server",
		"description": "Connect to Notion",
		"keywords": ["notes", "wiki"],
		"url": "https://mcp.notion.com/mcp",
		"auth": "dcr"
	}`

	m, err := loadManifestFromString(json)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if m.Kind != KindMcpServer {
		t.Errorf("kind = %q, want mcp_server", m.Kind)
	}

	entry, err := m.ToRegistryEntry()
	if err != nil {
		t.Fatalf("to registry entry: %v", err)
	}
	if entry.Kind != extensions.KindMcpServer {
		t.Errorf("entry kind = %v, want McpServer", entry.Kind)
	}
	if entry.Source.URL != "https://mcp.notion.com/mcp" {
		t.Errorf("source url = %q", entry.Source.URL)
	}
}

func TestMcpManifestMissingURL(t *testing.T) {
	json := `{
		"name": "broken",
		"display_name": "Broken",
		"kind": "mcp_server",
		"description": "No URL"
	}`

	m, err := loadManifestFromString(json)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	_, err = m.ToRegistryEntry()
	if err == nil {
		t.Error("expected error for MCP manifest without url")
	}
}

func TestManifestWithDownloadURL(t *testing.T) {
	json := `{
		"name": "gmail",
		"display_name": "Gmail",
		"kind": "tool",
		"version": "0.1.0",
		"description": "Gmail tool",
		"source": {
			"dir": "tools-src/gmail",
			"capabilities": "gmail-tool.capabilities.json",
			"crate_name": "gmail-tool"
		},
		"artifacts": {
			"wasm32-wasip2": {
				"url": "https://example.com/gmail.tar.gz",
				"sha256": null
			}
		}
	}`

	m, err := loadManifestFromString(json)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	entry, err := m.ToRegistryEntry()
	if err != nil {
		t.Fatalf("to registry entry: %v", err)
	}
	if entry.Source.Type != "wasm_download" {
		t.Errorf("source type = %q, want wasm_download", entry.Source.Type)
	}
	if entry.FallbackSource == nil {
		t.Error("expected fallback source")
	}
	if entry.FallbackSource.Type != "wasm_buildable" {
		t.Errorf("fallback type = %q, want wasm_buildable", entry.FallbackSource.Type)
	}
}

func TestLoadBundles(t *testing.T) {
	json := `{
		"bundles": {
			"google": {
				"display_name": "Google Suite",
				"description": "All Google tools",
				"extensions": ["tools/gmail", "tools/google-calendar"],
				"shared_auth": "google_oauth_token",
				"aliases": ["gws", "gsuite"]
			}
		}
	}`

	bf, err := loadBundlesFromString(json)
	if err != nil {
		t.Fatalf("load bundles: %v", err)
	}
	if len(bf.Bundles) != 1 {
		t.Fatalf("bundles = %d, want 1", len(bf.Bundles))
	}
	b := bf.Bundles["google"]
	if b.DisplayName != "Google Suite" {
		t.Errorf("display_name = %q", b.DisplayName)
	}
	if b.SharedAuth != "google_oauth_token" {
		t.Errorf("shared_auth = %q", b.SharedAuth)
	}
	if len(b.Aliases) != 2 {
		t.Errorf("aliases = %d, want 2", len(b.Aliases))
	}
}

func TestCatalog(t *testing.T) {
	c := NewCatalog()

	c.Register(&ExtensionManifest{
		Name: "github", DisplayName: "GitHub", Kind: KindTool,
		Description: "GitHub integration", Keywords: []string{"dev"},
	})
	c.Register(&ExtensionManifest{
		Name: "slack", DisplayName: "Slack", Kind: KindChannel,
		Description: "Slack integration", Keywords: []string{"messaging"},
	})

	if len(c.List()) != 2 {
		t.Errorf("list = %d, want 2", len(c.List()))
	}

	tools := c.ListByKind(KindTool)
	if len(tools) != 1 || tools[0].Name != "github" {
		t.Errorf("tools = %v, want [github]", tools)
	}

	results := c.Search("slack")
	if len(results) != 1 {
		t.Errorf("search slack = %d, want 1", len(results))
	}

	results = c.Search("integration")
	if len(results) != 2 {
		t.Errorf("search integration = %d, want 2", len(results))
	}
}

func TestCatalogBundleExtensions(t *testing.T) {
	c := NewCatalog()
	c.Register(&ExtensionManifest{
		Name: "gmail", DisplayName: "Gmail", Kind: KindTool,
		Description: "Gmail tool",
	})
	c.Register(&ExtensionManifest{
		Name: "google-calendar", DisplayName: "Google Calendar", Kind: KindTool,
		Description: "Calendar tool",
	})
	c.bundles["google"] = BundleDefinition{
		DisplayName: "Google Suite",
		Extensions:  []string{"tools/gmail", "tools/google-calendar"},
	}

	exts, err := c.BundleExtensions("google")
	if err != nil {
		t.Fatalf("bundle extensions: %v", err)
	}
	if len(exts) != 2 {
		t.Errorf("exts = %d, want 2", len(exts))
	}
}

func TestDiscoverManifests(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "slack.json"), []byte(`{
		"name": "slack",
		"display_name": "Slack",
		"kind": "tool",
		"description": "Slack tool"
	}`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "invalid.txt"), []byte("not json"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"invalid": true}`), 0644)

	manifests, err := DiscoverManifests(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(manifests) != 1 {
		t.Errorf("manifests = %d, want 1", len(manifests))
	}
	if manifests[0].Name != "slack" {
		t.Errorf("name = %q, want slack", manifests[0].Name)
	}
}

func TestManifestKindToExtensionKind(t *testing.T) {
	cases := []struct {
		mk   ManifestKind
		want extensions.ExtensionKind
	}{
		{KindTool, extensions.KindWasmTool},
		{KindChannel, extensions.KindWasmChannel},
		{KindMcpServer, extensions.KindMcpServer},
		{ManifestKind("unknown"), extensions.KindWasmTool},
	}
	for _, tc := range cases {
		if got := tc.mk.ToExtensionKind(); got != tc.want {
			t.Errorf("%q -> %v, want %v", tc.mk, got, tc.want)
		}
	}
}
