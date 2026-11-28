package flags_test

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imlargo/medusa/flags"
)

func fixed(values map[string]flags.Flag) flags.Loader {
	return func(context.Context) (map[string]flags.Flag, error) { return values, nil }
}

func loaded(t *testing.T, values map[string]flags.Flag) *flags.Flags {
	t.Helper()
	f := flags.New(fixed(values))
	if err := f.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh() = %v", err)
	}
	return f
}

func TestAFlagIsOnForEveryoneForAListOrForNobody(t *testing.T) {
	f := loaded(t, map[string]flags.Flag{
		"on":      {Enabled: true},
		"off":     {},
		"allowed": {Allow: []string{"acme"}},
	})
	ctx := t.Context()

	tests := []struct {
		name, key string
		want      bool
	}{
		{"on", "anyone", true},
		{"on", "", true},
		{"off", "anyone", false},
		{"allowed", "acme", true},
		{"allowed", "globex", false},
		{"never_defined", "anyone", false},
	}
	for _, tt := range tests {
		if got := f.Enabled(ctx, tt.name, tt.key); got != tt.want {
			t.Errorf("Enabled(%q, %q) = %v, want %v", tt.name, tt.key, got, tt.want)
		}
	}
}

// A rollout that moved a caller back and forth between two code paths would
// be worse than no rollout: the same key always gets the same answer, and a
// higher percentage only adds keys.
func TestAPercentageIsStableAndOnlyGrows(t *testing.T) {
	const keys = 10000
	on := func(percentage int) map[string]bool {
		f := loaded(t, map[string]flags.Flag{"new_checkout": {Percentage: percentage}})
		set := map[string]bool{}
		for i := range keys {
			key := "tenant-" + strconv.Itoa(i)
			if f.Enabled(t.Context(), "new_checkout", key) {
				set[key] = true
			}
		}
		return set
	}

	at10, again, at50 := on(10), on(10), on(50)

	if len(at10) < keys*8/100 || len(at10) > keys*12/100 {
		t.Errorf("10%% turned on %d of %d keys, want about 1000", len(at10), keys)
	}
	if len(at50) < keys*47/100 || len(at50) > keys*53/100 {
		t.Errorf("50%% turned on %d of %d keys, want about 5000", len(at50), keys)
	}
	for key := range at10 {
		if !again[key] {
			t.Fatalf("%s was on and then off at the same percentage", key)
		}
		if !at50[key] {
			t.Fatalf("%s was on at 10%% and off at 50%%: raising a rollout must only add keys", key)
		}
	}

	none, all := on(0), on(100)
	if len(none) != 0 || len(all) != keys {
		t.Errorf("0%% and 100%% turned on %d and %d keys, want 0 and %d", len(none), len(all), keys)
	}
}

// Two flags at the same percentage must not pick the same callers, or the
// first customers would get every experiment at once.
func TestDifferentFlagsPickDifferentKeys(t *testing.T) {
	f := loaded(t, map[string]flags.Flag{"a": {Percentage: 50}, "b": {Percentage: 50}})

	same := 0
	const keys = 2000
	for i := range keys {
		key := strconv.Itoa(i)
		if f.Enabled(t.Context(), "a", key) == f.Enabled(t.Context(), "b", key) {
			same++
		}
	}
	if same > keys*60/100 {
		t.Errorf("two flags agreed on %d of %d keys, want about half: they share one rollout", same, keys)
	}
}

// When the store cannot be read, the answer must not change: the last values
// stay, and before any load the defaults do.
func TestALoaderFailureKeepsTheValuesInUse(t *testing.T) {
	var fail atomic.Bool
	loader := func(context.Context) (map[string]flags.Flag, error) {
		if fail.Load() {
			return nil, errors.New("database is down")
		}
		return map[string]flags.Flag{"new_checkout": {Enabled: true}}, nil
	}

	fail.Store(true)
	f := flags.New(loader, flags.WithDefaults(map[string]flags.Flag{"safe_default": {Enabled: true}}))
	ctx := t.Context()

	if err := f.Refresh(ctx); err == nil {
		t.Fatal("Refresh() = nil, want the loader's error")
	}
	if !f.Enabled(ctx, "safe_default", "") || f.Enabled(ctx, "new_checkout", "") {
		t.Error("before any load the defaults must be in use, and nothing else")
	}

	fail.Store(false)
	if err := f.Refresh(ctx); err != nil {
		t.Fatalf("Refresh() = %v", err)
	}
	if !f.Enabled(ctx, "new_checkout", "") {
		t.Error("new_checkout is off after a successful load")
	}

	fail.Store(true)
	_ = f.Refresh(ctx)
	if !f.Enabled(ctx, "new_checkout", "") {
		t.Error("new_checkout turned off when the store became unreachable, want the last value kept")
	}
}

func TestStartPicksUpAChangeWithoutARestart(t *testing.T) {
	var enabled atomic.Bool
	f := flags.New(func(context.Context) (map[string]flags.Flag, error) {
		return map[string]flags.Flag{"new_checkout": {Enabled: enabled.Load()}}, nil
	}, flags.WithRefresh(10*time.Millisecond), flags.WithLogger(slog.New(slog.DiscardHandler)))

	done := make(chan error, 1)
	go func() { done <- f.Start(context.Background()) }()

	enabled.Store(true)
	deadline := time.Now().Add(5 * time.Second)
	for !f.Enabled(t.Context(), "new_checkout", "") {
		if time.Now().After(deadline) {
			t.Fatal("the change never reached the running process")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := f.Stop(t.Context()); err != nil {
		t.Errorf("Stop() = %v", err)
	}
	if err := <-done; err != nil {
		t.Errorf("Start() = %v, want nil after Stop", err)
	}
}

func TestStopBeforeStartDoesNothing(t *testing.T) {
	if err := flags.New(fixed(nil)).Stop(t.Context()); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}
