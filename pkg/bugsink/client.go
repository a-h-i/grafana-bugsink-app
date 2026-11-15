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

	c := &Client{authHeader: "Bearer " + token}
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

// ListProjects returns every project the token can see, following next until it is null.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	var cursor *string
	for {
		resp, err := c.gen.ProjectsList(ctx, &gen.ProjectsListParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		var page gen.PaginatedProjectListList
		if err := decodeJSON(resp, &page); err != nil {
			return nil, err
		}
		if page.Results == nil {
			return nil, fmt.Errorf("%w: %w", ErrNotBugsink, &APIError{Status: resp.StatusCode})
		}
		for _, p := range page.Results {
			projects = append(projects, Project{ID: derefInt(p.Id), Name: p.Name})
		}
		if page.Next == nil || *page.Next == "" {
			return projects, nil
		}
		nextCursor, err := cursorOf(*page.Next)
		if err != nil {
			return nil, err
		}
		if cursor != nil && *cursor == nextCursor {
			return nil, fmt.Errorf("%w: next page repeats the cursor", ErrNotBugsink)
		}
		cursor = &nextCursor
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

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
