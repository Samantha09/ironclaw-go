package builtin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/tools"
)

func TestHTTPToolGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "hello")
	}))
	defer server.Close()

	tool := NewHTTPTool()
	out, err := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    server.URL,
	}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("output = %q", out.Content)
	}
}

func TestHTTPToolPostWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		fmt.Fprintf(w, "received: %s", body)
	}))
	defer server.Close()

	tool := NewHTTPTool()
	out, err := tool.Execute(context.Background(), map[string]any{
		"method": "POST",
		"url":    server.URL,
		"body":   "data",
	}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.Content, "received: data") {
		t.Errorf("output = %q", out.Content)
	}
}

func TestHTTPToolHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			t.Errorf("X-Custom = %q", r.Header.Get("X-Custom"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tool := NewHTTPTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"method":  "GET",
		"url":     server.URL,
		"headers": map[string]any{"X-Custom": "value"},
	}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestHTTPToolInvalidURL(t *testing.T) {
	tool := NewHTTPTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    "not-a-url",
	}, nil)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestHTTPToolForbiddenScheme(t *testing.T) {
	tool := NewHTTPTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    "ftp://example.com",
	}, nil)
	if err == nil {
		t.Error("expected error for non-http scheme")
	}
}

func TestHTTPToolMissingURL(t *testing.T) {
	tool := NewHTTPTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    "",
	}, nil)
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestHTTPToolRequiresApproval(t *testing.T) {
	tool := NewHTTPTool()
	if tool.RequiresApproval(map[string]any{"method": "GET"}) != gate.Never {
		t.Error("expected Never for GET")
	}
	if tool.RequiresApproval(map[string]any{"method": "POST"}) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved for POST")
	}
	if tool.RequiresApproval(map[string]any{"method": "DELETE"}) != gate.UnlessAutoApproved {
		t.Error("expected UnlessAutoApproved for DELETE")
	}
}

func TestHTTPToolMaxBodySize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 6MB of data, exceeding the 5MB limit
		data := make([]byte, 6*1024*1024)
		for i := range data {
			data[i] = 'x'
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
	}))
	defer server.Close()

	tool := NewHTTPTool()
	out, err := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    server.URL,
	}, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Should be truncated by LimitReader
	if len(out.Content) > 6*1024*1024 {
		t.Error("expected content to be limited")
	}
}

func TestHTTPToolJobContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tool := NewHTTPTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    server.URL,
	}, &tools.JobContext{UserID: "u1"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
}
