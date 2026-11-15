// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"sync"
)

type flightCall[V any] struct {
	done      chan struct{}
	val       V
	err       error
	dups      int
	abandoned bool // the leader's context ended or fn panicked: followers must call again
}

// flight shares one call among concurrent callers of the same key and remembers nothing once it ends.
type flight[V any] struct {
	mu    sync.Mutex
	calls map[string]*flightCall[V]
}

func newFlight[V any]() *flight[V] {
	return &flight[V]{calls: make(map[string]*flightCall[V])}
}

// do runs fn once for concurrent callers of one key; after a cancelled shared call a caller with a live context runs fn alone.
func (g *flight[V]) do(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error) {
	for {
		g.mu.Lock()
		c, shared := g.calls[key]
		if !shared {
			c = &flightCall[V]{done: make(chan struct{})}
			g.calls[key] = c
			g.mu.Unlock()
			return g.lead(ctx, key, c, fn)
		}
		c.dups++
		g.mu.Unlock()
		select {
		case <-c.done:
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
		if !c.abandoned {
			return c.val, c.err
		}
		if err := ctx.Err(); err != nil {
			var zero V
			return zero, err
		}
	}
}

func (g *flight[V]) lead(ctx context.Context, key string, c *flightCall[V], fn func(context.Context) (V, error)) (V, error) {
	c.abandoned = true
	defer func() {
		g.mu.Lock()
		delete(g.calls, key)
		g.mu.Unlock()
		close(c.done)
	}()
	c.val, c.err = fn(ctx)
	c.abandoned = c.err != nil && ctx.Err() != nil
	return c.val, c.err
}
