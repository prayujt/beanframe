package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func records(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	var result []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		result = append(result, record)
	}
	return result
}

func TestContextInheritanceAndIsolation(t *testing.T) {
	var output bytes.Buffer
	l, err := New(Params{Level: slog.LevelDebug, Format: "json", Output: &output})
	if err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent := WithAttrs(base, "request_id", "request-1", slog.String("service", "mcp"))
	left := WithAttrs(parent, "tool", "list_accounts")
	right := WithAttrs(parent, "tool", "get_balances")
	cancel()
	if left.Err() != context.Canceled {
		t.Fatal("derived context lost cancellation")
	}
	l.DebugCtx(left, "left", "count", 2)
	l.InfoCtx(right, "right")
	l.WarnCtx(parent, "parent")
	l.Error("plain", "error", "example")
	got := records(t, &output)
	if len(got) != 4 {
		t.Fatalf("got %d records", len(got))
	}
	for i, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		if got[i]["level"] != level {
			t.Fatalf("wrong level: %v", got[i])
		}
	}
	if got[0]["tool"] != "list_accounts" || got[0]["count"] != float64(2) || got[1]["tool"] != "get_balances" {
		t.Fatal(got)
	}
	for _, record := range got[:3] {
		if record["request_id"] != "request-1" || record["service"] != "mcp" {
			t.Fatal(record)
		}
	}
	if _, ok := got[2]["tool"]; ok {
		t.Fatal("child fields leaked into parent")
	}
	if _, ok := got[3]["request_id"]; ok {
		t.Fatal("context leaked into plain log")
	}
}

func TestConcurrentContexts(t *testing.T) {
	var output bytes.Buffer
	l, err := New(Params{Format: "json", Output: &output})
	if err != nil {
		t.Fatal(err)
	}
	parent := WithAttrs(context.Background(), "service", "mcp")
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := WithAttrs(parent, "request_id", fmt.Sprint(i))
			l.InfoCtx(ctx, fmt.Sprint(i))
		}()
	}
	wg.Wait()
	got := records(t, &output)
	if len(got) != 100 {
		t.Fatal(len(got))
	}
	for _, record := range got {
		if record["request_id"] != record["msg"] || record["service"] != "mcp" {
			t.Fatal(record)
		}
	}
}

func TestGlobalLoggerAndSlogAdapter(t *testing.T) {
	previous := shared.Load()
	t.Cleanup(func() { shared.Store(previous) })
	var output bytes.Buffer
	if err := Configure(Params{Level: slog.LevelWarn, Format: "json", Output: &output}); err != nil {
		t.Fatal(err)
	}
	ctx := WithAttrs(context.Background(), "request_id", "request-2")
	Debug("filtered")
	Info("filtered")
	DebugCtx(ctx, "filtered")
	InfoCtx(ctx, "filtered")
	Warn("plain warning")
	Error("plain error")
	WarnCtx(ctx, "context warning")
	ErrorCtx(ctx, "context error")
	Slog().With("component", "sdk").WithGroup("details").WarnContext(ctx, "sdk", "count", 3)
	got := records(t, &output)
	if len(got) != 5 {
		t.Fatalf("level filtering failed: %v", got)
	}
	if got[2]["request_id"] != "request-2" || got[3]["request_id"] != "request-2" {
		t.Fatal(got)
	}
	details, ok := got[4]["details"].(map[string]any)
	if !ok || details["request_id"] != "request-2" || details["count"] != float64(3) || got[4]["component"] != "sdk" {
		t.Fatal(got[4])
	}
	configured := Default()
	if err := Configure(Params{Format: "invalid"}); err == nil {
		t.Fatal("accepted invalid format")
	}
	if Default() != configured {
		t.Fatal("invalid configuration replaced the configured logger")
	}
}

func TestDefaultOutputAndIndependentLogger(t *testing.T) {
	previous := Default()
	var output bytes.Buffer
	l, err := New(Params{Output: &output})
	if err != nil {
		t.Fatal(err)
	}
	l.DebugCtx(WithAttrs(context.Background(), "request_id", "text-1"), "hello", "count", 2)
	if !strings.Contains(output.String(), "level=DEBUG") || !strings.Contains(output.String(), "request_id=text-1") || !strings.Contains(output.String(), "count=2") {
		t.Fatal(output.String())
	}
	if Default() != previous {
		t.Fatal("New changed global logger")
	}
}
