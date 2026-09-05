package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RunWithLease executes work while this backend owns the Redis lease. The work
// context is cancelled immediately when the parent is cancelled or ownership
// can no longer be renewed. Callers must use that context for database and
// network operations and keep irreversible operations idempotent: a lease is
// coordination, not a substitute for database-level fencing.
//
// A backend started without Redis may execute locally only when the shared
// runtime is optional (the single-instance debug fallback). Required/release
// runtimes fail closed whenever Redis cannot grant or retain the lease.
func RunWithLease(ctx context.Context, name string, ttl time.Duration, work func(context.Context) error) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errors.New("scheduler lease name is required")
	}
	if work == nil {
		return false, errors.New("scheduler lease work is required")
	}

	lease, acquired, err := AcquireLease(ctx, name, ttl)
	if err != nil {
		if errors.Is(err, ErrUnavailable) && !Required() {
			return true, work(ctx)
		}
		return false, fmt.Errorf("acquire scheduler lease %q: %w", name, err)
	}
	if !acquired {
		return false, nil
	}

	// Keep leases alive for work that legitimately exceeds its original TTL.
	// The compare-and-renew Lua script guarantees that a stale owner can never
	// extend a lease which has already been replaced by another instance.
	workContext, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	heartbeatContext, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() {
		interval := lease.ttl / 3
		if interval < 250*time.Millisecond {
			interval = 250 * time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatContext.Done():
				heartbeatDone <- nil
				return
			case <-ticker.C:
				renewContext, cancel := context.WithTimeout(heartbeatContext, minDuration(2*time.Second, interval))
				err := lease.Renew(renewContext)
				cancel()
				if err != nil {
					if heartbeatContext.Err() != nil {
						heartbeatDone <- nil
						return
					}
					// Stop the lease-protected work before returning the ownership
					// failure. Every scheduler callback is required to use this
					// context for its blocking operations.
					cancelWork()
					heartbeatDone <- fmt.Errorf("renew scheduler lease %q: %w", name, err)
					return
				}
			}
		}
	}()

	var workErr, heartbeatErr, releaseErr error
	// Cleanup is deferred inside a small scope so a callback panic cannot leave
	// the heartbeat renewing an abandoned lease until the parent process exits.
	// The panic still propagates after ownership is released; this layer only
	// guarantees the lease lifecycle.
	func() {
		defer func() {
			stopHeartbeat()
			heartbeatErr = <-heartbeatDone
			releaseContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			releaseErr = lease.Release(releaseContext)
			cancel()
		}()
		workErr = work(workContext)
	}()
	if releaseErr != nil {
		releaseErr = fmt.Errorf("release scheduler lease %q: %w", name, releaseErr)
	}
	return true, errors.Join(workErr, heartbeatErr, releaseErr)
}

func minDuration(first, second time.Duration) time.Duration {
	if first < second {
		return first
	}
	return second
}
