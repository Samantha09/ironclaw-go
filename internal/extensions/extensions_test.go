package extensions

import (
	"errors"
	"testing"
)

func TestExtensionKindString(t *testing.T) {
	cases := []struct {
		kind ExtensionKind
		want string
	}{
		{KindMcpServer, "mcp_server"},
		{KindWasmTool, "wasm_tool"},
		{KindWasmChannel, "wasm_channel"},
		{KindChannelRelay, "channel_relay"},
		{KindAcpAgent, "acp_agent"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.kind.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRegistryEntryValidate(t *testing.T) {
	cases := []struct {
		name    string
		entry   RegistryEntry
		wantErr bool
	}{
		{"ok", RegistryEntry{Name: "test", Kind: KindWasmTool}, false},
		{"missing name", RegistryEntry{Kind: KindWasmTool}, true},
		{"missing kind", RegistryEntry{Name: "test"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.entry.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestEntryRegistry(t *testing.T) {
	r := NewEntryRegistry()

	entry := RegistryEntry{
		Name:        "slack",
		DisplayName: "Slack",
		Kind:        KindWasmChannel,
		Description: "Slack integration",
		Source:      ExtensionSource{Type: "mcp_url", URL: "https://slack.example.com"},
		AuthHint:    AuthHint{Type: AuthHintOAuthPreConfigured, SetupURL: "https://api.slack.com/apps"},
	}

	if err := r.Register(entry); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, ok := r.Get("slack")
	if !ok {
		t.Fatal("expected to find slack")
	}
	if got.Name != "slack" {
		t.Errorf("name = %q, want slack", got.Name)
	}

	list := r.List()
	if len(list) != 1 {
		t.Errorf("list = %d, want 1", len(list))
	}

	byKind := r.ListByKind(KindWasmChannel)
	if len(byKind) != 1 {
		t.Errorf("byKind = %d, want 1", len(byKind))
	}
}

func TestEntryRegistryInvalidEntry(t *testing.T) {
	r := NewEntryRegistry()
	if err := r.Register(RegistryEntry{}); err == nil {
		t.Error("expected error for invalid entry")
	}
}

func TestAuthResult(t *testing.T) {
	cases := []struct {
		name string
		ar   AuthResult
		auth bool
		tok  bool
	}{
		{"authenticated", AuthResult{Status: AuthStatusAuthenticated}, true, false},
		{"no_auth", AuthResult{Status: AuthStatusNoAuthRequired}, false, false},
		{"awaiting_token", AuthResult{Status: AuthStatusAwaitingToken}, false, true},
		{"needs_setup", AuthResult{Status: AuthStatusNeedsSetup}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ar.IsAuthenticated() != tc.auth {
				t.Errorf("IsAuthenticated() = %v, want %v", tc.ar.IsAuthenticated(), tc.auth)
			}
			if tc.ar.IsAwaitingToken() != tc.tok {
				t.Errorf("IsAwaitingToken() = %v, want %v", tc.ar.IsAwaitingToken(), tc.tok)
			}
		})
	}
}

func TestEnsureReadyOutcome(t *testing.T) {
	ready := ReadyOutcome("test", KindWasmTool, PhaseReady, nil)
	if ready.OutcomeType != "ready" {
		t.Errorf("type = %q, want ready", ready.OutcomeType)
	}
	if ready.PhaseValue() != PhaseReady {
		t.Errorf("phase = %q, want ready", ready.PhaseValue())
	}

	needsAuth := NeedsAuthOutcome("test", KindMcpServer, PhaseNeedsAuth, AuthResult{Status: AuthStatusAwaitingAuthorization}, "token")
	if needsAuth.OutcomeType != "needs_auth" {
		t.Errorf("type = %q, want needs_auth", needsAuth.OutcomeType)
	}
	if needsAuth.Auth == nil {
		t.Error("expected auth")
	}

	needsSetup := NeedsSetupOutcome("test", KindWasmTool, PhaseNeedsSetup, "Configure OAuth", "https://setup.example.com")
	if needsSetup.OutcomeType != "needs_setup" {
		t.Errorf("type = %q, want needs_setup", needsSetup.OutcomeType)
	}
	if needsSetup.Instructions != "Configure OAuth" {
		t.Errorf("instructions = %q, want 'Configure OAuth'", needsSetup.Instructions)
	}
}

func TestInstalledExtensionDefaults(t *testing.T) {
	e := NewInstalledExtension("echo", KindWasmTool)
	if e.Name != "echo" {
		t.Errorf("name = %q, want echo", e.Name)
	}
	if !e.Installed {
		t.Error("expected Installed to default to true")
	}
}

func TestExtensionErrors(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrNotFound("foo"), "extension not_found foo: extension not found"},
		{ErrAlreadyInstalled("bar"), "extension already_installed bar: extension already installed"},
		{ErrAuthFailed("baz", errors.New("bad token")), "extension auth_failed baz: bad token"},
		{ErrInstallFailed("x", errors.New("disk full")), "extension install_failed x: disk full"},
	}
	for _, tc := range cases {
		t.Run(tc.want[:20], func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtensionErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := ErrAuthFailed("test", inner)
	if !errors.Is(err, inner) {
		t.Error("expected errors.Is to match inner error")
	}
}
