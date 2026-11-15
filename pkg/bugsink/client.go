// SPDX-License-Identifier: GPL-3.0-or-later

// Package bugsink is a small client for the Bugsink REST API.
package bugsink

import (
	"context"
	"encoding/json"
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

// New builds a client for the Bugsink at baseURL using httpClient for transport.
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
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), apiPath)

	c := &Client{authHeader: "Bearer " + token, baseURL: u.String()}
	c.gen, err = gen.NewClient(u.String(),
		gen.WithHTTPClient(httpClient),
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
	_, err := listPages(ctx, func(cursor *string) (*http.Response, error) {
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

// listPages requests pages with the cursor taken from next until next is null, onPage returns true or MaxPages is reached.
func listPages[T any](ctx context.Context, fetch func(cursor *string) (*http.Response, error),
	onPage func(results []T) (stop bool)) (truncated bool, err error) {
	var cursor *string
	for pages := 1; ; pages++ {
		resp, err := fetch(cursor)
		if err != nil {
			return false, err
		}
		var body struct {
			Next    *string `json:"next"`
			Results []T     `json:"results"`
		}
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
		if pages >= MaxPages {
			return true, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		cursor = &next
	}
}

// GetIssue reads one issue and discards it; it exists to probe the issues:read capability.
func (c *Client) GetIssue(ctx context.Context, id string) error {
	resp, err := c.gen.IssuesRetrieve(ctx, id)
	return expectSuccess(resp, err)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	if json.Unmarshal(body, out) != nil {
		return fmt.Errorf("%w: %w", ErrNotBugsink, &APIError{Status: resp.StatusCode})
	}
	return nil
}

func readAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	return newAPIError(resp.StatusCode, body)
}
