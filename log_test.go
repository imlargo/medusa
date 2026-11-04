package medusa_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/imlargo/medusa"
)

func TestLogHandlerAddsTheRequestID(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(medusa.LogHandler(slog.NewJSONHandler(&out, nil))).With("service", "orders")

	logger.InfoContext(medusa.WithRequestID(context.Background(), "abc"), "created")
	logger.InfoContext(context.Background(), "tick")

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %s", len(lines), out.String())
	}

	var inRequest, outside map[string]any
	_ = json.Unmarshal(lines[0], &inRequest)
	_ = json.Unmarshal(lines[1], &outside)

	if inRequest["request_id"] != "abc" || inRequest["service"] != "orders" {
		t.Errorf("in a request: %s, want request_id abc and the logger's attributes", lines[0])
	}
	if _, ok := outside["request_id"]; ok {
		t.Errorf("outside a request: %s, want no request_id", lines[1])
	}
}
