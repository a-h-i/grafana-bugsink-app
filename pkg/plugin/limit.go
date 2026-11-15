// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import "net/http"

// maxConcurrentCalls is the most Bugsink requests one data source instance has waiting for an answer at once.
const maxConcurrentCalls = 8

type limitedTransport struct {
	next  http.RoundTripper
	slots chan struct{}
}

// limitTransport lets at most n requests wait for an answer at once; a queued request ends with its context's error.
func limitTransport(next http.RoundTripper, n int) http.RoundTripper {
	return &limitedTransport{next: next, slots: make(chan struct{}, n)}
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case t.slots <- struct{}{}:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	defer func() { <-t.slots }()
	return t.next.RoundTrip(req)
}

func (t *limitedTransport) CloseIdleConnections() {
	if c, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
