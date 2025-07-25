// Copyright 2013 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"fmt"
	"time"

	gh "github.com/google/go-github/v73/github"
)

// GistsService handles communication with the Gist related
// methods of the GitHub API.
//
// GitHub API docs: https://docs.github.com/rest/gists
type GistsService struct {
	client *gh.Client
}

func NewGistsService(client *gh.Client) *GistsService {
	return &GistsService{client: client}
}

// Gist represents a GitHub's gist.
type Gist struct {
	ID          *string                   `json:"id,omitempty"`
	Description *string                   `json:"description,omitempty"`
	Public      *bool                     `json:"public,omitempty"`
	Owner       *gh.User                  `json:"owner,omitempty"`
	Files       map[GistFilename]GistFile `json:"files,omitempty"`
	Comments    *int                      `json:"comments,omitempty"`
	HTMLURL     *string                   `json:"html_url,omitempty"`
	GitPullURL  *string                   `json:"git_pull_url,omitempty"`
	GitPushURL  *string                   `json:"git_push_url,omitempty"`
	CreatedAt   *gh.Timestamp             `json:"created_at,omitempty"`
	UpdatedAt   *gh.Timestamp             `json:"updated_at,omitempty"`
	NodeID      *string                   `json:"node_id,omitempty"`
	History     []*GistCommit             `json:"history,omitempty"`
}

// GetID returns the ID field if it's non-nil, zero value otherwise.
func (g *Gist) GetID() string {
	if g == nil || g.ID == nil {
		return ""
	}
	return *g.ID
}

func (g *Gist) GetHistory() []*GistCommit {
	if g == nil || g.History == nil {
		return nil
	}
	return g.History
}

func (g *Gist) String() string {
	return gh.Stringify(g)
}

// GistFilename represents filename on a gist.
type GistFilename string

// GistFile represents a file on a gist.
type GistFile struct {
	Size     *int    `json:"size,omitempty"`
	Filename *string `json:"filename,omitempty"`
	Language *string `json:"language,omitempty"`
	Type     *string `json:"type,omitempty"`
	RawURL   *string `json:"raw_url,omitempty"`
	Content  *string `json:"content,omitempty"`
}

func (g GistFile) String() string {
	return gh.Stringify(g)
}

// GistCommit represents a commit on a gist.
type GistCommit struct {
	URL          *string         `json:"url,omitempty"`
	Version      *string         `json:"version,omitempty"`
	User         *gh.User        `json:"user,omitempty"`
	ChangeStatus *gh.CommitStats `json:"change_status,omitempty"`
	CommittedAt  *gh.Timestamp   `json:"committed_at,omitempty"`
	NodeID       *string         `json:"node_id,omitempty"`
}

// GetVersion returns the Version field if it's non-nil, zero value otherwise.
func (g *GistCommit) GetVersion() string {
	if g == nil || g.Version == nil {
		return ""
	}
	return *g.Version
}

func (gc GistCommit) String() string {
	return gh.Stringify(gc)
}

// GistFork represents a fork of a gist.
type GistFork struct {
	URL       *string       `json:"url,omitempty"`
	User      *gh.User      `json:"user,omitempty"`
	ID        *string       `json:"id,omitempty"`
	CreatedAt *gh.Timestamp `json:"created_at,omitempty"`
	UpdatedAt *gh.Timestamp `json:"updated_at,omitempty"`
	NodeID    *string       `json:"node_id,omitempty"`
}

func (gf GistFork) String() string {
	return gh.Stringify(gf)
}

// GistListOptions specifies the optional parameters to the
// GistsService.List, GistsService.ListAll, and GistsService.ListStarred methods.
type GistListOptions struct {
	// Since filters Gists by time.
	Since time.Time `url:"since,omitempty"`

	gh.ListOptions
}

// List gists for a user. Passing the empty string will list
// all public gists if called anonymously. However, if the call
// is authenticated, it will returns all gists for the authenticated
// user.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#list-gists-for-a-user
// GitHub API docs: https://docs.github.com/rest/gists/gists#list-gists-for-the-authenticated-user
//
//meta:operation GET /gists
//meta:operation GET /users/{username}/gists
func (s *GistsService) List(ctx context.Context, user string, opts *GistListOptions) ([]*Gist, *gh.Response, error) {
	var u string
	if user != "" {
		u = fmt.Sprintf("users/%v/gists", user)
	} else {
		u = "gists"
	}
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var gists []*Gist
	resp, err := s.client.Do(ctx, req, &gists)
	if err != nil {
		return nil, resp, err
	}

	return gists, resp, nil
}

// ListAll lists all public gists.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#list-public-gists
//
//meta:operation GET /gists/public
func (s *GistsService) ListAll(ctx context.Context, opts *GistListOptions) ([]*Gist, *gh.Response, error) {
	u, err := addOptions("gists/public", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var gists []*Gist
	resp, err := s.client.Do(ctx, req, &gists)
	if err != nil {
		return nil, resp, err
	}

	return gists, resp, nil
}

// ListStarred lists starred gists of authenticated user.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#list-starred-gists
//
//meta:operation GET /gists/starred
func (s *GistsService) ListStarred(ctx context.Context, opts *GistListOptions) ([]*Gist, *gh.Response, error) {
	u, err := addOptions("gists/starred", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var gists []*Gist
	resp, err := s.client.Do(ctx, req, &gists)
	if err != nil {
		return nil, resp, err
	}

	return gists, resp, nil
}

// Get a single gist.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#get-a-gist
//
//meta:operation GET /gists/{gist_id}
func (s *GistsService) Get(ctx context.Context, id string) (*Gist, *gh.Response, error) {
	u := fmt.Sprintf("gists/%v", id)
	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	gist := new(Gist)
	resp, err := s.client.Do(ctx, req, gist)
	if err != nil {
		return nil, resp, err
	}

	return gist, resp, nil
}

// GetRevision gets a specific revision of a gist.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#get-a-gist-revision
//
//meta:operation GET /gists/{gist_id}/{sha}
func (s *GistsService) GetRevision(ctx context.Context, id, sha string) (*Gist, *gh.Response, error) {
	u := fmt.Sprintf("gists/%v/%v", id, sha)
	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	gist := new(Gist)
	resp, err := s.client.Do(ctx, req, gist)
	if err != nil {
		return nil, resp, err
	}

	return gist, resp, nil
}

// Create a gist for authenticated user.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#create-a-gist
//
//meta:operation POST /gists
func (s *GistsService) Create(ctx context.Context, gist *Gist) (*Gist, *gh.Response, error) {
	u := "gists"
	req, err := s.client.NewRequest("POST", u, gist)
	if err != nil {
		return nil, nil, err
	}

	g := new(Gist)
	resp, err := s.client.Do(ctx, req, g)
	if err != nil {
		return nil, resp, err
	}

	return g, resp, nil
}

// Edit a gist.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#update-a-gist
//
//meta:operation PATCH /gists/{gist_id}
func (s *GistsService) Edit(ctx context.Context, id string, gist *Gist) (*Gist, *gh.Response, error) {
	u := fmt.Sprintf("gists/%v", id)
	req, err := s.client.NewRequest("PATCH", u, gist)
	if err != nil {
		return nil, nil, err
	}

	g := new(Gist)
	resp, err := s.client.Do(ctx, req, g)
	if err != nil {
		return nil, resp, err
	}

	return g, resp, nil
}

// ListCommits lists commits of a gist.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#list-gist-commits
//
//meta:operation GET /gists/{gist_id}/commits
func (s *GistsService) ListCommits(ctx context.Context, id string, opts *gh.ListOptions) ([]*GistCommit, *gh.Response, error) {
	u := fmt.Sprintf("gists/%v/commits", id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var gistCommits []*GistCommit
	resp, err := s.client.Do(ctx, req, &gistCommits)
	if err != nil {
		return nil, resp, err
	}

	return gistCommits, resp, nil
}

// Delete a gist.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#delete-a-gist
//
//meta:operation DELETE /gists/{gist_id}
func (s *GistsService) Delete(ctx context.Context, id string) (*gh.Response, error) {
	u := fmt.Sprintf("gists/%v", id)
	req, err := s.client.NewRequest("DELETE", u, nil)
	if err != nil {
		return nil, err
	}

	return s.client.Do(ctx, req, nil)
}

// Star a gist on behalf of authenticated user.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#star-a-gist
//
//meta:operation PUT /gists/{gist_id}/star
func (s *GistsService) Star(ctx context.Context, id string) (*gh.Response, error) {
	u := fmt.Sprintf("gists/%v/star", id)
	req, err := s.client.NewRequest("PUT", u, nil)
	if err != nil {
		return nil, err
	}

	return s.client.Do(ctx, req, nil)
}

// Unstar a gist on a behalf of authenticated user.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#unstar-a-gist
//
//meta:operation DELETE /gists/{gist_id}/star
func (s *GistsService) Unstar(ctx context.Context, id string) (*gh.Response, error) {
	u := fmt.Sprintf("gists/%v/star", id)
	req, err := s.client.NewRequest("DELETE", u, nil)
	if err != nil {
		return nil, err
	}

	return s.client.Do(ctx, req, nil)
}

// IsStarred checks if a gist is starred by authenticated user.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#check-if-a-gist-is-starred
//
//meta:operation GET /gists/{gist_id}/star
func (s *GistsService) IsStarred(ctx context.Context, id string) (bool, *gh.Response, error) {
	u := fmt.Sprintf("gists/%v/star", id)
	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return false, nil, err
	}

	resp, err := s.client.Do(ctx, req, nil)
	starred, err := parseBoolResponse(err)
	return starred, resp, err
}

// Fork a gist.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#fork-a-gist
//
//meta:operation POST /gists/{gist_id}/forks
func (s *GistsService) Fork(ctx context.Context, id string) (*Gist, *gh.Response, error) {
	u := fmt.Sprintf("gists/%v/forks", id)
	req, err := s.client.NewRequest("POST", u, nil)
	if err != nil {
		return nil, nil, err
	}

	g := new(Gist)
	resp, err := s.client.Do(ctx, req, g)
	if err != nil {
		return nil, resp, err
	}

	return g, resp, nil
}

// ListForks lists forks of a gist.
//
// GitHub API docs: https://docs.github.com/rest/gists/gists#list-gist-forks
//
//meta:operation GET /gists/{gist_id}/forks
func (s *GistsService) ListForks(ctx context.Context, id string, opts *gh.ListOptions) ([]*GistFork, *gh.Response, error) {
	u := fmt.Sprintf("gists/%v/forks", id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var gistForks []*GistFork
	resp, err := s.client.Do(ctx, req, &gistForks)
	if err != nil {
		return nil, resp, err
	}

	return gistForks, resp, nil
}
