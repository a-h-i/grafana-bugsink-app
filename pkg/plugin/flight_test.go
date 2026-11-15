// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// dupsOf is the number of followers waiting on the key's call.
func (g *flight[V]) dupsOf(key string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.calls[key]; ok {
		return c.dups
	}
	return 0
}

// totalDups is the number of followers waiting on any call.
func (g *flight[V]) totalDups() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, c := range g.calls {
		n += c.dups
	}
	return n
}

type flightResult struct {
	val int
	err error
}

func TestFlightSharesOneCall(t *testing.T) {
	g := newFlight[int]()
	var runs atomic.Int64
	release := make(chan struct{})
	fn := func(context.Context) (int, error) { runs.Add(1); <-release; return 42, nil }
	results := make(chan flightResult, 8)
	call := func() { v, err := g.do(context.Background(), "k", fn); results <- flightResult{v, err} }
	go call()
	eventually(t, "the leader running", func() bool { return runs.Load() == 1 })
	for i := 0; i < 7; i++ {
		go call()
	}
	eventually(t, "7 followers", func() bool { return g.dupsOf("k") == 7 })
	close(release)
	for i := 0; i < 8; i++ {
		if r := <-results; r.val != 42 || r.err != nil {
			t.Fatalf("got %v", r)
		}
	}
	if n := runs.Load(); n != 1 {
		t.Fatalf("fn ran %d times", n)
	}
}

func TestFlightKeysAreSeparate(t *testing.T) {
	g := newFlight[string]()
	var wg sync.WaitGroup
	for _, key := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, _ := g.do(context.Background(), key, func(context.Context) (string, error) { return key, nil })
			if v != key {
				t.Errorf("key %q got %q", key, v)
			}
		}()
	}
	wg.Wait()
}

func TestFlightSharesErrorsOnce(t *testing.T) {
	g := newFlight[int]()
	boom := errors.New("boom")
	var runs atomic.Int64
	release := make(chan struct{})
	results := make(chan flightResult, 4)
	for i := 0; i < 4; i++ {
		go func() {
			v, err := g.do(context.Background(), "k", func(context.Context) (int, error) {
				runs.Add(1)
				<-release
				return 0, boom
			})
			results <- flightResult{v, err}
		}()
	}
	eventually(t, "3 followers", func() bool { return g.dupsOf("k") == 3 })
	close(release)
	for i := 0; i < 4; i++ {
		if r := <-results; !errors.Is(r.err, boom) {
			t.Fatalf("got %v", r)
		}
	}
	v, err := g.do(context.Background(), "k", func(context.Context) (int, error) { runs.Add(1); return 7, nil })
	if v != 7 || err != nil || runs.Load() != 2 {
		t.Fatalf("next call: %d %v after %d runs", v, err, runs.Load())
	}
}

func TestFlightCancelledLeader(t *testing.T) {
	g := newFlight[int]()
	var runs atomic.Int64
	release := make(chan struct{})
	fn := func(ctx context.Context) (int, error) {
		n := runs.Add(1)
		if n == 1 {
			<-release
			return 0, ctx.Err()
		}
		return 99, nil
	}
	leaderCtx, cancel := context.WithCancel(context.Background())
	leader := make(chan flightResult, 1)
	follower := make(chan flightResult, 1)
	go func() { v, err := g.do(leaderCtx, "k", fn); leader <- flightResult{v, err} }()
	eventually(t, "the leader running", func() bool { return runs.Load() == 1 })
	go func() { v, err := g.do(context.Background(), "k", fn); follower <- flightResult{v, err} }()
	eventually(t, "the follower waiting", func() bool { return g.dupsOf("k") == 1 })
	cancel()
	close(release)
	if r := <-leader; !errors.Is(r.err, context.Canceled) {
		t.Fatalf("leader got %v", r)
	}
	if r := <-follower; r.val != 99 || r.err != nil {
		t.Fatalf("follower got %v", r)
	}
	if n := runs.Load(); n != 2 {
		t.Fatalf("fn ran %d times, want 2", n)
	}
}

func TestFlightFollowerCancelled(t *testing.T) {
	g := newFlight[int]()
	var runs atomic.Int64
	release := make(chan struct{})
	fn := func(context.Context) (int, error) { runs.Add(1); <-release; return 5, nil }
	leader := make(chan flightResult, 1)
	go func() { v, err := g.do(context.Background(), "k", fn); leader <- flightResult{v, err} }()
	eventually(t, "the leader running", func() bool { return runs.Load() == 1 })
	ctx, cancel := context.WithCancel(context.Background())
	follower := make(chan flightResult, 1)
	go func() { v, err := g.do(ctx, "k", fn); follower <- flightResult{v, err} }()
	eventually(t, "the follower waiting", func() bool { return g.dupsOf("k") == 1 })
	cancel()
	if r := <-follower; !errors.Is(r.err, context.Canceled) {
		t.Fatalf("follower got %v", r)
	}
	close(release)
	if r := <-leader; r.val != 5 || r.err != nil {
		t.Fatalf("leader got %v", r)
	}
}

func TestFlightLeaderPanic(t *testing.T) {
	g := newFlight[int]()
	func() {
		defer func() { _ = recover() }()
		_, _ = g.do(context.Background(), "k", func(context.Context) (int, error) { panic("boom") })
	}()
	v, err := g.do(context.Background(), "k", func(context.Context) (int, error) { return 3, nil })
	if v != 3 || err != nil {
		t.Fatalf("call after a panic: %d %v", v, err)
	}
}
