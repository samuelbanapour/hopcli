package core

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"
)

// diskFullPollInterval is how often a paused transaction checks whether the
// disk has room again. Short enough that freeing space (deleting a file,
// emptying Trash) is noticed quickly; long enough not to hammer the disk.
// A var, not a const, so tests can shrink it.
var diskFullPollInterval = 3 * time.Second

// InsufficientSpaceError is hop's own preflight refusal to start a download
// or extraction that cannot possibly fit, as opposed to an ENOSPC surfaced
// mid-write by the kernel. Both mean the same thing to a caller: the disk is
// full right now, not that anything is wrong with the artifact.
type InsufficientSpaceError struct {
	Need, Free int64
	Path       string
}

func (e *InsufficientSpaceError) Error() string {
	return fmt.Sprintf("need %s but only %s is free on %s", humanBytes(e.Need), humanBytes(e.Free), e.Path)
}

// isDiskFull reports whether err means "the disk is full", covering both an
// ENOSPC that bubbled up from a write/mkdir syscall and hop's own preflight
// check. This is the one failure Apply pauses for instead of aborting the
// transaction: unlike a bad checksum or a 404, it is something the user can
// fix without losing any work already done.
func isDiskFull(err error) bool {
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	var full *InsufficientSpaceError
	return errors.As(err, &full)
}

// retryUntilSpace runs op once. If it fails with a disk-full condition, it
// logs that the transaction is paused, then polls until the condition clears
// (or ctx is cancelled) before retrying — the download or extraction itself
// is retried from scratch, but nothing already materialised in this
// transaction is discarded. Any other error is returned immediately.
func retryUntilSpace(ctx context.Context, sink Sink, name string, op func() error) error {
	if sink == nil {
		sink = DiscardSink
	}
	warned := false
	for {
		err := op()
		if err == nil || !isDiskFull(err) {
			return err
		}
		if ctx.Err() != nil {
			return err
		}
		if !warned {
			sink.Log("⏸ paused: out of disk space installing %s (%v)", name, err)
			sink.Log("  free some space — hop will pick up where it left off automatically")
			warned = true
		}
		select {
		case <-time.After(diskFullPollInterval):
		case <-ctx.Done():
			return err
		}
	}
}
