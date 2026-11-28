// Package flags answers "is this turned on, for this one?": feature flags
// that change without a deploy, for releasing a change to some callers
// before all of them.
//
//	features := flags.New(store.Load,
//	    flags.WithDefaults(map[string]flags.Flag{"new_checkout": {Percentage: 0}}))
//	// run it with lifecycle, so it keeps itself up to date
//
//	if features.Enabled(ctx, "new_checkout", tenantID) {
//	    return h.newCheckout(ctx, in)
//	}
//	return h.oldCheckout(ctx, in)
//
// A flag is on for everyone, for a percentage, or for a list of keys. The key
// is whatever the rollout is by: a tenant, a user, a source. The same key
// always gets the same answer for a flag, so a caller does not flip between
// the old path and the new one from one request to the next, and raising the
// percentage only ever adds callers.
//
// Flags are read from a loader, such as a table, and kept in memory:
// evaluating one costs no I/O. If the loader fails, the last values it gave
// stay in use; before it has ever succeeded, the defaults do. A flag nobody
// defined is off.
//
// A flag is temporary. When a change is on for everyone, delete the flag and
// the old path with it.
package flags

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultRefresh is how often flags are read again from the loader.
const DefaultRefresh = 30 * time.Second

// Flag is one feature's rollout.
type Flag struct {
	// Enabled turns the flag on for everyone, whatever else is set.
	Enabled bool

	// Percentage of keys the flag is on for, from 0 to 100. Which keys is
	// decided by a hash of the flag's name and the key, so it is stable, and
	// different for each flag.
	Percentage int

	// Allow lists keys the flag is always on for: the team's own account,
	// the first customers to try a change.
	Allow []string
}

// on reports whether the flag is on for key.
func (f Flag) on(name, key string) bool {
	if f.Enabled || slices.Contains(f.Allow, key) {
		return true
	}
	if f.Percentage <= 0 || key == "" {
		return false
	}
	return bucket(name, key) < f.Percentage
}

// bucket places key in one of 100 buckets for the flag name.
func bucket(name, key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % 100)
}

// Loader reads every flag from where they are kept.
type Loader func(ctx context.Context) (map[string]Flag, error)

// Flags evaluates feature flags from a snapshot it keeps up to date. It
// satisfies lifecycle.Server: Start refreshes the snapshot until Stop.
type Flags struct {
	load     Loader
	refresh  time.Duration
	logger   *slog.Logger
	snapshot atomic.Pointer[map[string]Flag]

	mu   sync.Mutex
	stop context.CancelFunc
	done chan struct{}
}

// Option configures Flags.
type Option func(*Flags)

// WithDefaults sets the flags in use until the loader first succeeds, and
// for a process that never starts refreshing. Give every flag the value that
// is safe when nothing is known: usually off.
func WithDefaults(defaults map[string]Flag) Option {
	return func(f *Flags) { f.snapshot.Store(&defaults) }
}

// WithRefresh sets how often flags are read again, [DefaultRefresh] by
// default. It is how long a change takes to reach every process.
func WithRefresh(interval time.Duration) Option {
	return func(f *Flags) { f.refresh = interval }
}

// WithLogger sets where a failed refresh is logged. The default is
// slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(f *Flags) { f.logger = logger }
}

// New evaluates the flags load returns. Nothing is loaded yet: call Refresh
// once at startup to begin with current values, and run Start to keep them
// current.
func New(load Loader, opts ...Option) *Flags {
	empty := map[string]Flag{}
	f := &Flags{load: load, refresh: DefaultRefresh, logger: slog.Default()}
	f.snapshot.Store(&empty)
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Enabled reports whether the flag name is on for key. key is what the
// rollout is by; pass "" for a flag that is on or off for everyone. A flag
// that is not defined is off.
func (f *Flags) Enabled(_ context.Context, name, key string) bool {
	return (*f.snapshot.Load())[name].on(name, key)
}

// Refresh reads the flags again now. On an error the values in use stay as
// they are.
func (f *Flags) Refresh(ctx context.Context) error {
	loaded, err := f.load(ctx)
	if err != nil {
		return fmt.Errorf("flags: refresh: %w", err)
	}
	if loaded == nil {
		loaded = map[string]Flag{}
	}
	f.snapshot.Store(&loaded)
	return nil
}

// Start refreshes the flags every interval until Stop is called or ctx is
// canceled. A refresh that fails is logged, and the flags in use stay.
func (f *Flags) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	defer close(done)

	f.mu.Lock()
	if f.stop != nil {
		f.mu.Unlock()
		cancel()
		return errors.New("flags: already started")
	}
	f.stop, f.done = cancel, done
	f.mu.Unlock()

	ticker := time.NewTicker(f.refresh)
	defer ticker.Stop()

	for {
		if err := f.Refresh(ctx); err != nil && ctx.Err() == nil {
			f.logger.WarnContext(ctx, "feature flags could not be refreshed; the last values stay in use", slog.Any("error", err))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Stop ends the refreshing and waits for it, until ctx is done. Calling it
// before Start does nothing.
func (f *Flags) Stop(ctx context.Context) error {
	f.mu.Lock()
	stop, done := f.stop, f.done
	f.mu.Unlock()
	if stop == nil {
		return nil
	}

	stop()
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}
