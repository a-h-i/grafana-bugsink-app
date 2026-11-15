// SPDX-License-Identifier: GPL-3.0-or-later

// Package bugsink is a small client for the Bugsink REST API.
package bugsink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink/internal/gen"
)

const apiPath = "/api/canonical/0"

const maxBodyBytes = 10 << 20

type Project struct {
	ID   int
	Name string
}

// Client reads from one Bugsink instance with one API token.
type Client struct {
	gen        *gen.Client
	authHeader string
	baseURL    string
}

// New builds a client for the Bugsink at baseURL using httpClient for transport; it never follows a redirect.
func New(baseURL, token string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ErrInvalidURL
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNoToken
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	noRedirects := *httpClient
	noRedirects.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	noRedirects.Transport = redactingTransport{next: httpClient.Transport}
	if httpClient.Transport == nil {
		noRedirects.Transport = redactingTransport{next: http.DefaultTransport}
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), apiPath)

	c := &Client{authHeader: "Bearer " + token, baseURL: u.String()}
	c.gen, err = gen.NewClient(u.String(),
		gen.WithHTTPClient(queryHidingDoer{&noRedirects}),
		gen.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", c.authHeader)
			return nil
		}))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	return c, nil
}

// ListProjects returns the projects the token can see, following next until it is null or MaxPages is reached.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	_, err := listPages(ctx, MaxPages, func(cursor *string) (*http.Response, error) {
		return c.gen.ProjectsList(ctx, &gen.ProjectsListParams{Cursor: cursor})
	}, func(results []gen.ProjectList) bool {
		for _, p := range results {
			projects = append(projects, Project{ID: deref(p.Id), Name: p.Name})
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// BaseURL is the Bugsink address without the API path or a trailing slash.
func (c *Client) BaseURL() string { return c.baseURL }

// PageSize is the number of results per page of every Bugsink list.
const PageSize = 250

// pageBody is the envelope of every Bugsink list answer.
type pageBody[T any] struct {
	Next    *string `json:"next"`
	Results []T     `json:"results"`
}

// listPages requests pages with the cursor taken from next until next is null, onPage returns true or maxPages is reached.
func listPages[T any](ctx context.Context, maxPages int, fetch func(cursor *string) (*http.Response, error),
	onPage func(results []T) (stop bool)) (truncated bool, err error) {
	var cursor *string
	for pages := 1; ; pages++ {
		resp, err := fetch(cursor)
		if err != nil {
			return false, err
		}
		var body pageBody[T]
		if err := decodeJSON(resp, &body); err != nil {
			return false, err
		}
		if body.Results == nil {
			return false, fmt.Errorf("%w: %w", ErrNotBugsink, &APIError{Status: resp.StatusCode})
		}
		if onPage(body.Results) || body.Next == nil || *body.Next == "" {
			return false, nil
		}
		next, err := cursorOf(*body.Next)
		if err != nil {
			return false, err
		}
		if cursor != nil && *cursor == next {
			return false, fmt.Errorf("%w: next page repeats the cursor", ErrNotBugsink)
		}
		if pages >= maxPages {
			return true, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		cursor = &next
	}
}

// ListEventsWithoutIssue lists events with no issue filter; it exists to probe the events:read capability.
func (c *Client) ListEventsWithoutIssue(ctx context.Context) error {
	resp, err := c.gen.EventsList(ctx, &gen.EventsListParams{})
	return expectSuccess(resp, err)
}

// cursorOf returns the cursor query parameter of a next URL.
func cursorOf(next string) (string, error) {
	u, err := url.Parse(next)
	if err != nil || !u.Query().Has("cursor") {
		return "", fmt.Errorf("%w: next page has no cursor", ErrNotBugsink)
	}
	return u.Query().Get("cursor"), nil
}

func expectSuccess(resp *http.Response, err error) error {
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	return readAPIError(resp)
}

func decodeJSON(resp *http.Response, out any) error {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return readAPIError(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxBodyBytes {
		return ErrTooLarge
	}
	if json.Unmarshal(body, out) != nil {
		return fmt.Errorf("%w: %w", ErrNotBugsink, &APIError{Status: resp.StatusCode})
	}
	return nil
}

func readAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	var token string
	if resp.Request != nil {
		token = tokenOf(resp.Request)
	}
	return newAPIError(resp.StatusCode, body, token)
}

// tokenOf is the request's Authorization header without "Bearer ".
func tokenOf(req *http.Request) string {
	return strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
}

// queryHidingDoer replaces the URL in a request's url.Error by the request's scheme, host and path, made forwardable.
type queryHidingDoer struct{ client *http.Client }

func (d queryHidingDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.client.Do(req)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return resp, &url.Error{Op: urlErr.Op, URL: forwardableURL(req.URL, tokenOf(req)), Err: urlErr.Err}
	}
	return resp, err
}

// forwardableURL is u's scheme, host and path with token redacted, also in its path-escaped form, and cut to maxDetailChars.
func forwardableURL(u *url.URL, token string) string {
	bare := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	escaped := (&url.URL{Path: token}).EscapedPath()
	return forwardable(redact(bare.String(), escaped), token)
}

// redactingTransport makes every error of a request and of reading or closing its answer forwardable, and drops the answer's Location, which net/http quotes when it cannot parse it.
type redactingTransport struct{ next http.RoundTripper }

func (t redactingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token := tokenOf(req)
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return resp, forwardableError(err, token)
	}
	resp.Header.Del("Location")
	if resp.Body != nil {
		resp.Body = redactingBody{ReadCloser: resp.Body, token: token}
	}
	return resp, nil
}

type redactingBody struct {
	io.ReadCloser
	token string
}

func (b redactingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	return n, forwardableError(err, b.token)
}

func (b redactingBody) Close() error {
	return forwardableError(b.ReadCloser.Close(), b.token)
}

// forwardableError is err, or a new error with err's forwardable text when the text holds token or is too long.
func forwardableError(err error, token string) error {
	if err == nil {
		return nil
	}
	if text := forwardable(err.Error(), token); text != err.Error() {
		return errors.New(text)
	}
	return err
}
