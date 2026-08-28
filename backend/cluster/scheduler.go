package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RunWithLease executes work at most once across backend instances while the
// Redis lease is held. A backend started without Redis may execute locally only
// when the shared runtime is optional (the single-instance debug fallback).
// Required/release runtimes fail closed whenever Redis cannot grant the lease.
func RunWithLease(ctx context.Context, name string, ttl time.Duration, work func() error) (bool, error) {
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
			return true, work()
		}
		return false, fmt.Errorf("acquire scheduler lease %q: %w", name, err)
	}
	if !acquired {
		return false, nil
	}

	workErr := work()
	releaseContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	releaseErr := lease.Release(releaseContext)
	cancel()
	if releaseErr != nil {
		releaseErr = fmt.Errorf("release scheduler lease %q: %w", name, releaseErr)
	}
	return true, errors.Join(workErr, releaseErr)
}
