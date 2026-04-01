package sandbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/sandbox"
)

func TestRunnerReturnsMockResponse(t *testing.T) {
	t.Parallel()

	runner := sandbox.NewRunner(sandbox.MockExecutor{Response: "ok"}, time.Second)
	got, err := runner.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got != "ok" {
		t.Fatalf("response mismatch: got %q want %q", got, "ok")
	}
}

func TestRunnerTimeout(t *testing.T) {
	t.Parallel()

	runner := sandbox.NewRunner(sandbox.MockExecutor{Delay: 120 * time.Millisecond}, 20*time.Millisecond)
	_, err := runner.Run(context.Background(), "slow prompt")
	if !errors.Is(err, sandbox.ErrExecutionTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestRunnerErrorDoesNotLeakPrompt(t *testing.T) {
	t.Parallel()

	prompt := "sensitive-prompt"
	runner := sandbox.NewRunner(sandbox.MockExecutor{Err: errors.New("executor blew up for sensitive-prompt")}, time.Second)
	_, err := runner.Run(context.Background(), prompt)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), prompt) {
		t.Fatalf("error leaked prompt: %v", err)
	}
}

func TestSellerSafeViewIsRedacted(t *testing.T) {
	t.Parallel()

	view := sandbox.NewSellerSafeView()
	if view.Prompt != "[REDACTED]" || view.Response != "[REDACTED]" {
		t.Fatalf("seller view should be redacted: %+v", view)
	}
}
