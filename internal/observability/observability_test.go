package observability

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerLevels(t *testing.T) {
	t.Run("debug_level", func(t *testing.T) {
		var buf bytes.Buffer
		log := NewLoggerWithWriter(&buf, "debug")
		log.Debug("debug msg")
		log.Info("info msg")
		log.Warn("warn msg")

		out := buf.String()
		if !strings.Contains(out, "debug msg") {
			t.Error("expected debug msg in output")
		}
		if !strings.Contains(out, "info msg") {
			t.Error("expected info msg in output")
		}
	})

	t.Run("info_level_filters_debug", func(t *testing.T) {
		var buf bytes.Buffer
		log := NewLoggerWithWriter(&buf, "info")
		log.Debug("debug msg")
		log.Info("info msg")

		out := buf.String()
		if strings.Contains(out, "debug msg") {
			t.Error("expected debug msg to be filtered")
		}
		if !strings.Contains(out, "info msg") {
			t.Error("expected info msg in output")
		}
	})

	t.Run("error_level", func(t *testing.T) {
		var buf bytes.Buffer
		log := NewLoggerWithWriter(&buf, "error")
		log.Info("info msg")
		log.Error("error msg", nil)

		out := buf.String()
		if strings.Contains(out, "info msg") {
			t.Error("expected info msg to be filtered")
		}
		if !strings.Contains(out, "error msg") {
			t.Error("expected error msg in output")
		}
	})
}

func TestLoggerWith(t *testing.T) {
	var buf bytes.Buffer
	log := NewLoggerWithWriter(&buf, "info")
	scoped := log.With(slog.String("component", "test"))
	scoped.Info("scoped msg")

	out := buf.String()
	if !strings.Contains(out, "component=test") {
		t.Errorf("expected component attr, got: %s", out)
	}
	if !strings.Contains(out, "scoped msg") {
		t.Error("expected scoped msg")
	}
}

func TestLoggerError(t *testing.T) {
	var buf bytes.Buffer
	log := NewLoggerWithWriter(&buf, "info")
	log.Error("something failed", nil, slog.String("detail", "oom"))

	out := buf.String()
	if !strings.Contains(out, "something failed") {
		t.Error("expected msg")
	}
	if !strings.Contains(out, "detail=oom") {
		t.Error("expected detail attr")
	}
}

func TestTimer(t *testing.T) {
	var buf bytes.Buffer
	log := NewLoggerWithWriter(&buf, "info")
	stop := log.StartTimer("op")
	stop()

	out := buf.String()
	if !strings.Contains(out, "timer") {
		t.Error("expected timer log")
	}
	if !strings.Contains(out, "name=op") {
		t.Error("expected timer name")
	}
	if !strings.Contains(out, "duration=") {
		t.Error("expected duration")
	}
}
