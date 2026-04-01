package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrExecutionTimeout = errors.New("sandbox execution timed out")
	ErrEmptyPrompt      = errors.New("prompt is required")
	ErrExecutionFailed  = errors.New("sandbox execution failed")
)

type Executor interface {
	Execute(ctx context.Context, prompt string) (string, error)
}

type Runner struct {
	executor Executor
	timeout  time.Duration
}

func NewRunner(executor Executor, timeout time.Duration) *Runner {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Runner{
		executor: executor,
		timeout:  timeout,
	}
}

// Run executes a single request-scoped prompt with timeout.
// It keeps the execution contract minimal and does not expose filesystem,
// environment variables, or secret material to executors by design.
func (r *Runner) Run(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		return "", ErrEmptyPrompt
	}
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	response, err := r.executor.Execute(runCtx, prompt)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return "", ErrExecutionTimeout
		}
		// Never include prompt or response in returned errors.
		return "", ErrExecutionFailed
	}
	return response, nil
}

type MockExecutor struct {
	Response string
	Delay    time.Duration
	Err      error
}

func (m MockExecutor) Execute(ctx context.Context, prompt string) (string, error) {
	if m.Delay > 0 {
		timer := time.NewTimer(m.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	if m.Err != nil {
		return "", m.Err
	}
	if m.Response == "" {
		return "mock-response", nil
	}
	return m.Response, nil
}

type SellerSafeView struct {
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
}

func NewSellerSafeView() SellerSafeView {
	return SellerSafeView{
		Prompt:   "[REDACTED]",
		Response: "[REDACTED]",
	}
}
