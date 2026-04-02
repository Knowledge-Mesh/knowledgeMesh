package control

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHandleControlPing(t *testing.T) {
	t.Parallel()
	b, err := handleControl(context.Background(), []byte(`{"type":"ping"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["type"] != "pong" {
		t.Fatalf("expected pong, got %v", out["type"])
	}
	if out["ok"] != true {
		t.Fatalf("expected ok true")
	}
}

func TestHandleControlEmptyBody(t *testing.T) {
	t.Parallel()
	b, err := handleControl(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Fatalf("expected ok true")
	}
}
