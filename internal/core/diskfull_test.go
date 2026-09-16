package core

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"testing"
	"time"
)

func TestIsDiskFull(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"raw ENOSPC", syscall.ENOSPC, true},
		{"wrapped ENOSPC", fmt.Errorf("creating staging directory: %w", &pathErrLike{syscall.ENOSPC}), true},
		{"insufficient space", &InsufficientSpaceError{Need: 10, Free: 1, Path: "/tmp"}, true},
		{"wrapped insufficient space", fmt.Errorf("downloading: %w", &InsufficientSpaceError{Need: 10, Free: 1}), true},
		{"unrelated error", errors.New("boom"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isDiskFull(c.err); got != c.want {
				t.Errorf("isDiskFull(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// pathErrLike mimics the Unwrap shape of *os.PathError without pulling one
// in, so the ENOSPC-wrapping case can be tested without touching the
// filesystem.
type pathErrLike struct{ err error }

func (p *pathErrLike) Error() string { return "path error: " + p.err.Error() }
func (p *pathErrLike) Unwrap() error { return p.err }

func TestRetryUntilSpace_PausesThenSucceeds(t *testing.T) {
	diskFullPollInterval = 5 * time.Millisecond
	defer func() { diskFullPollInterval = 3 * time.Second }()

	attempts := 0
	err := retryUntilSpace(context.Background(), DiscardSink, "widget", func() error {
		attempts++
		if attempts < 3 {
			return syscall.ENOSPC
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryUntilSpace: unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryUntilSpace_NonDiskFullFailsImmediately(t *testing.T) {
	attempts := 0
	want := errors.New("digest mismatch")
	err := retryUntilSpace(context.Background(), DiscardSink, "widget", func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected immediate passthrough of %v, got %v", want, err)
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt for a non-disk-full error, got %d", attempts)
	}
}

func TestRetryUntilSpace_StopsOnCancel(t *testing.T) {
	diskFullPollInterval = 5 * time.Millisecond
	defer func() { diskFullPollInterval = 3 * time.Second }()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := retryUntilSpace(ctx, DiscardSink, "widget", func() error {
		return syscall.ENOSPC
	})
	if !isDiskFull(err) {
		t.Fatalf("expected the last disk-full error back after cancellation, got %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("expected context to be cancelled")
	}
}
