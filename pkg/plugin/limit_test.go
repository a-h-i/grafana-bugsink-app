// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// holdingServer holds every request until release is closed and counts how many it holds.
type holdingServer struct {
	srv         *httptest.Server
	release     chan struct{}
	held, max   atomic.Int64
	reached     atomic.Int64
	releaseOnce sync.Once
}

func newHoldingServer(t *testing.T) *holdingServer {
	t.Helper()
	h := &holdingServer{release: make(chan struct{})}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h.reached.Add(1)
		n := h.held.Add(1)
		for {
			m := h.max.Load()
			if n <= m || h.max.CompareAndSwap(m, n) {
				break
			}
		}
		<-h.release
		h.held.Add(-1)
	}))
	t.Cleanup(func() { h.open(); h.srv.Close() })
	return h
}

func (h *holdingServer) open() { h.releaseOnce.Do(func() { close(h.release) }) }

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// queuedInLimit counts the goroutines waiting for a slot of a limit transport.
func queuedInLimit() int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, " [select") && strings.Contains(g, "(*limitedTransport).RoundTrip(") {
			n++
		}
	}
	return n
}

func get(ctx context.Context, rt http.RoundTripper, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return rt.RoundTrip(req)
}

func TestLimitTransport(t *testing.T) {
	h := newHoldingServer(t)
	rt := limitTransport(http.DefaultTransport, 2)
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			resp, err := get(context.Background(), rt, h.srv.URL)
			if err == nil {
				_ = resp.Body.Close()
			}
			errs <- err
		}()
	}
	eventually(t, "two held requests", func() bool { return h.held.Load() == 2 })
	time.Sleep(50 * time.Millisecond)
	if m := h.max.Load(); m != 2 {
		t.Fatalf("the server held %d requests at once, want 2", m)
	}
	h.open()
	for i := 0; i < 5; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}
}

func TestLimitTransportQueuedCancel(t *testing.T) {
	h := newHoldingServer(t)
	rt := limitTransport(http.DefaultTransport, 1)
	first := make(chan error, 1)
	go func() {
		resp, err := get(context.Background(), rt, h.srv.URL)
		if err == nil {
			_ = resp.Body.Close()
		}
		first <- err
	}()
	eventually(t, "the first request held", func() bool { return h.held.Load() == 1 })

	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() { _, err := get(ctx, rt, h.srv.URL); second <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-second; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued request returned %v, want context.Canceled", err)
	}
	if n := h.reached.Load(); n != 1 {
		t.Fatalf("the server saw %d requests, want 1", n)
	}
	h.open()
	if err := <-first; err != nil {
		t.Fatalf("first request failed: %v", err)
	}
}

func TestLimitTransportReleasesOnError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	rt := limitTransport(http.DefaultTransport, 1)
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := get(ctx, rt, url)
		cancel()
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("request %d: %v, want a connection error", i, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLimitTransportReleasesOnPanic(t *testing.T) {
	rt := limitTransport(roundTripFunc(func(*http.Request) (*http.Response, error) { panic("boom") }), 1)
	for i := 0; i < 3; i++ {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("the panic was swallowed")
				}
			}()
			_, _ = get(context.Background(), rt, "http://unused.invalid")
		}()
	}
}

func TestLimitTransportReleasesOnCancelledCall(t *testing.T) {
	h := newHoldingServer(t)
	rt := limitTransport(http.DefaultTransport, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := get(ctx, rt, h.srv.URL); done <- err }()
	eventually(t, "the request held", func() bool { return h.held.Load() == 1 })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled call returned %v", err)
	}
	h.open()
	resp, err := get(context.Background(), rt, h.srv.URL)
	if err != nil {
		t.Fatalf("the slot was not released: %v", err)
	}
	_ = resp.Body.Close()
}

func TestLimitTransportServesQueueInOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	gate := make(chan struct{})
	rt := limitTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		order = append(order, r.URL.Path)
		mu.Unlock()
		<-gate
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}), 1)
	var wg sync.WaitGroup
	start := func(path string) {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = get(context.Background(), rt, "http://x.invalid"+path) }()
	}
	start("/0")
	eventually(t, "the first request in", func() bool { mu.Lock(); defer mu.Unlock(); return len(order) == 1 })
	for i, p := range []string{"/1", "/2", "/3"} {
		start(p)
		eventually(t, "request "+p+" queued", func() bool { return queuedInLimit() == i+1 })
	}
	close(gate)
	wg.Wait()
	want := []string{"/0", "/1", "/2", "/3"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("served %v, want %v", order, want)
		}
	}
}
