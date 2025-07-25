package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v73/github"
	gists "github.com/open-policy-agent/rego-playground/internal/github"
	log "github.com/sirupsen/logrus"
)

const (
	readme = `# This is a Rego Playground Share`
)

type UnauthorizedError struct {
	Message          string `json:"message"`
	InvalidPrincipal bool   `json:"invalidPrincipal"`
}

func NewUnauthorizedError(message string, invalidPrincipal bool) error {
	return &UnauthorizedError{
		Message:          message,
		InvalidPrincipal: invalidPrincipal,
	}
}

func (e *UnauthorizedError) Error() string {
	return fmt.Sprintf("unauthorized: %s", e.Message)
}

func (e *UnauthorizedError) Is(target error) bool {
	var err *UnauthorizedError
	return errors.As(target, &err)
}

type RateLimitedError struct {
	Message string     `json:"message"`
	Reset   *time.Time `json:"reset,omitempty"`
}

func NewRateLimitedError(message string, reset *time.Time) error {
	return &RateLimitedError{
		Message: message,
		Reset:   reset,
	}
}

func (e *RateLimitedError) Error() string {
	if e.Reset == nil {
		return fmt.Sprintf("rate-limited: %s", e.Message)
	}
	return fmt.Sprintf("rate-limited: %s reset-time: %v", e.Message, e.Reset)
}

func (e *RateLimitedError) Is(target error) bool {
	var err *RateLimitedError
	return errors.As(target, &err)
}

type GistStore struct {
	baseUrl     *url.URL // Base URL for the Gist API, for debugging purposes
	externalURL string   // Used to add playground share link into README.md
	watchers    map[string]update
	wl          sync.Mutex
}

func NewGistStore(options ...GistStoreOption) *GistStore {
	s := &GistStore{
		watchers: make(map[string]update),
	}
	for _, opt := range options {
		opt(s)
	}
	return s
}

type GistStoreOption func(*GistStore)

func GistStoreExternalURL(u string) GistStoreOption {
	return func(s *GistStore) {
		s.externalURL = u
	}
}

func GistStoreBaseUrl(baseUrl *url.URL) GistStoreOption {
	return func(s *GistStore) {
		s.baseUrl = baseUrl
	}
}

type metadata struct {
	Coverage    bool `json:"coverage"`
	RegoVersion int  `json:"rego_version"`
}

func (m *metadata) toJSON() string {
	j, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(j)
}

func (m *metadata) updateDataRequest(dr *DataRequest) {
	if m == nil || dr == nil {
		return
	}

	dr.Coverage = m.Coverage
	dr.RegoVersion = &m.RegoVersion
}

func metadataFromDataRequest(dr *DataRequest) *metadata {
	meta := &metadata{}

	if dr == nil {
		return meta
	}

	meta.Coverage = dr.Coverage

	if dr.RegoVersion != nil {
		meta.RegoVersion = *dr.RegoVersion
	} else {
		meta.RegoVersion = 1 // Default to Rego version 1 if not specified
	}

	return meta
}

func metadataFromJSON(data []byte) (*metadata, error) {
	var m metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &m, nil
}

func (s *GistStore) Get(key *StoreKey, principal *Principal) (DataRequest, bool, error) {
	if principal == nil {
		return DataRequest{}, false, NewUnauthorizedError("authentication required to get a gist", true)
	}

	ctx := context.Background()

	gist, rev, err := s.getGist(ctx, key.Id, key.Revision, principal, true)
	if err != nil {
		return DataRequest{}, false, err
	}

	if gist == nil {
		log.Debugf("Gist not found for prefix: %v", key)
		return DataRequest{}, false, nil
	}

	log.Debugf("Gist found: %s", gist.GetID())

	dr := DataRequest{
		Etag: rev,
	}

	if gist.Files != nil {
		dr.RegoModules = make(map[string]interface{})
		for name, file := range gist.Files {
			if file.Content != nil && strings.HasSuffix(string(name), ".rego") {
				dr.RegoModules[string(name)] = *file.Content
			}
		}

		if file, ok := gist.Files["input.json"]; ok && file.Content != nil {
			if err := json.Unmarshal([]byte(*file.Content), &dr.Input); err != nil {
				log.Debugf("Failed to unmarshal input for gist %v: %v", key, err)
				return DataRequest{}, false, fmt.Errorf("failed to unmarshal input: %w", err)
			}
		}

		if file, ok := gist.Files["data.json"]; ok && file.Content != nil {
			if err := json.Unmarshal([]byte(*file.Content), &dr.Data); err != nil {
				log.Debugf("Failed to unmarshal data for gist %v: %v", key, err)
				return DataRequest{}, false, fmt.Errorf("failed to unmarshal data: %w", err)
			}
		}

		if file, ok := gist.Files["rego_playground_metadata.json"]; ok && file.Content != nil {
			meta, err := metadataFromJSON([]byte(*file.Content))
			if err != nil {
				log.Debugf("Failed to unmarshal metadata for gist %v: %v", key, err)
				return DataRequest{}, false, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
			meta.updateDataRequest(&dr)
		} else {
			log.Debugf("No metadata found for gist: %v", key)
		}
	} else {
		log.Debugf("Gist has no files: %v", key)
		return DataRequest{}, false, fmt.Errorf("gist %s found but has no files", key)
	}

	return dr, true, nil
}

func (s *GistStore) Put(key *StoreKey, dr DataRequest, principal *Principal) (*StoreKey, error) {
	if principal == nil {
		return nil, NewUnauthorizedError("authentication required to upsert a gist", true)
	}

	// TODO: Take ctx as arg?

	ctx := context.Background()

	if key != nil && key.Id != "" {
		gist, rev, err := s.patchGist(ctx, principal, key, &dr)
		if err != nil {
			return nil, err
		}

		if gist != nil {
			// A Gist was updated, notify any watcher
			s.wl.Lock()
			defer s.wl.Unlock()
			if up, ok := s.watchers[key.Id]; ok {
				up.cb(dr)
				delete(s.watchers, key.Id)
				close(up.done)
			}
		} else {
			// Not found, create a new gist
			return s.createNewGist(ctx, principal, &dr)
		}

		return &StoreKey{
			Id:       gist.GetID(),
			Revision: rev,
			KeyType:  KeyTypeGist,
		}, nil
	}

	return s.createNewGist(ctx, principal, &dr)
}

func (s *GistStore) createNewGist(ctx context.Context, principal *Principal, dr *DataRequest) (*StoreKey, error) {
	gist, rev, err := s.createGist(ctx, principal, dr)
	if err != nil {
		log.Debugf("Failed to create gist: %v", err)
		return nil, err
	}

	if updatedGist := s.patchReadme(ctx, gist, principal); updatedGist != nil {
		gist = updatedGist
	}

	commit, err := s.getLatestGistCommit(ctx, gist, principal)
	if err == nil && commit != nil {
		rev = commit.GetVersion()
	} else {
		log.Debugf("Failed to get latest gist commit for %s: %v", gist.GetID(), err)
		rev = "" // No revision available
	}

	return &StoreKey{
		Id:       gist.GetID(),
		Revision: rev,
		KeyType:  KeyTypeGist,
	}, nil
}

func (s *GistStore) patchReadme(ctx context.Context, gist *gists.Gist, principal *Principal) *gists.Gist {
	// Create a key with only the gist ID
	key := StoreKey{
		Id:      gist.GetID(),
		KeyType: KeyTypeGist,
	}
	oKey, err := key.toOpaque()
	if err != nil {
		log.Errorf("Failed to convert StoreKey to opaque: %v", err)
		return nil
	}

	// Update README.md to have a link to the new gist, initial gist creation will point to the old README with no link
	shareURL := fmt.Sprintf("%s/p/%s", strings.TrimSuffix(s.externalURL, "/"), oKey)

	finalReadme := fmt.Sprintf(`%s

[Show latest revision in Rego Playground](%s)`, readme, shareURL)

	log.Debugf("finalReadme: %s", finalReadme)

	gist.Files["README.md"] = gists.GistFile{
		Content: github.Ptr(finalReadme),
	}

	client := s.getClient(ctx, principal)
	updatedGist, _, err := client.Edit(context.Background(), gist.GetID(), gist)
	if err != nil {
		log.Errorf("Failed to patch gist %s with updated README: %v", gist.GetID(), err)
		return nil
	}

	return updatedGist
}

func (s *GistStore) Watch(key *StoreKey, etag string, timeout time.Duration, cb func(DataRequest), principal *Principal) (bool, error) {
	if principal == nil {
		return false, fmt.Errorf("authentication required to get a gist")
	}

	dr, found, err := s.Get(key, principal)
	if err != nil {
		return false, err
	}

	if !found {
		return false, nil
	}

	if dr.Etag != etag {
		log.Debugf("Etag for gist %s doesn't match", key.Id)
		go cb(dr)
	} else {
		done := make(chan struct{})

		log.Debugf("Watching gist %s for changes", key.Id)

		// Create a watcher that will notify when the gist is updated
		s.wl.Lock()
		// FIXME: this behavior is copied from the in-mem and s3 stores, but new requests will replace old watchers with the same bundle ID.
		// Should 'watchers' values be lists instead of single 'update' structs?
		s.watchers[key.Id] = update{
			cb:   cb,
			done: done,
		}
		s.wl.Unlock()

		go func() {
			select {
			case <-time.After(timeout):
				log.Debugf("Watcher for gist %s timed out", key.Id)
				s.wl.Lock()
				defer s.wl.Unlock()
				if up, ok := s.watchers[key.Id]; ok {
					dr, _, _ = s.Get(key, principal)
					up.cb(dr)
					delete(s.watchers, key.Id)
				}
			case <-done:
				log.Debugf("Watcher for gist %s triggered", key.Id)
				return
			}
		}()
	}

	return true, nil
}

func (s *GistStore) List(prefix *StoreKey, principal *Principal) ([]*StoreKey, error) {
	if principal == nil {
		return nil, NewUnauthorizedError("authentication required to list gists", false)
	}

	ctx := context.Background()

	gist, rev, err := s.getGist(ctx, prefix.Id, "", principal, true)
	if err != nil {
		return nil, err
	}

	if gist == nil {
		log.Debugf("Gist not found for prefix: %v", prefix)
		return nil, nil
	}

	log.Debugf("Gist found: %s", *gist.ID)
	return []*StoreKey{{Id: *gist.ID, Revision: rev}}, nil
}

func (s *GistStore) ListAll(_ *Principal) ([]*StoreKey, error) {
	return nil, nil
}

func (s *GistStore) getGist(ctx context.Context, id string, revision string, principal *Principal, fetchRevision bool) (*gists.Gist, string, error) {
	var client = s.getClient(ctx, principal)

	var gist *gists.Gist
	var resp *github.Response
	var err error

	if revision != "" {
		gist, resp, err = client.GetRevision(ctx, id, revision)
		log.Debugf("Get gist revision response: %v", resp)

		if resp != nil {
			if ok, err := checkResponseError(resp); !ok {
				return nil, "", err
			}
		}
	} else {
		gist, resp, err = client.Get(ctx, id)
		log.Debugf("Get gist response: %v", resp)

		if resp != nil {
			if ok, err := checkResponseError(resp); !ok {
				return nil, "", err
			}
		}

		if err != nil {
			return nil, "", fmt.Errorf("failed to get gist: %w", err)
		}

		if gist != nil && fetchRevision {
			rev, err := s.getLatestGistCommit(ctx, gist, principal)
			if err == nil && rev != nil {
				revision = rev.GetVersion()
			} else {
				log.Debugf("Failed to get latest gist commit for %s: %v", id, err)
				revision = "" // No revision available
			}
		}
	}

	return gist, revision, err
}

func checkResponseError(resp *github.Response) (bool, error) {
	if resp == nil {
		return true, nil
	}

	if resp.StatusCode == 403 {
		if rl := resp.Header.Get("x-ratelimit-remaining"); rl == "0" {
			var reset *time.Time
			if timestamp := resp.Header.Get("x-ratelimit-reset"); timestamp != "" {
				if i, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
					t := time.Unix(i, 0)
					reset = &t
				}
			}
			return false, NewRateLimitedError("rate limit exceeded", reset)
		}

		return false, NewUnauthorizedError("forbidden", false)
	}

	if resp.StatusCode == 404 {
		return false, nil
	}

	if resp.StatusCode == 422 {
		return false, NewRateLimitedError("spammed", nil)
	}

	return true, nil
}

func (s *GistStore) createGist(ctx context.Context, principal *Principal, dr *DataRequest) (*gists.Gist, string, error) {
	if principal == nil {
		return nil, "", fmt.Errorf("authentication required to create a gist")
	}

	finalReadme := readme + `
Check the second revision for the Playground share link`

	gist, err := updateGist(dr, &gists.Gist{
		Description: github.Ptr("Rego Playground"),
		Files: map[gists.GistFilename]gists.GistFile{
			"README.md": {
				Content: github.Ptr(finalReadme),
			},
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to update gist: %w", err)
	}

	client := s.getClient(ctx, principal)
	gist, resp, err := client.Create(ctx, gist)
	if resp != nil {
		if ok, err := checkResponseError(resp); !ok {
			return nil, "", err
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to create gist: %w", err)
	}

	log.Debugf("Create gist response: %v", resp)
	gj, _ := json.Marshal(gist)
	log.Debugf("Gist created: %s", gj)

	var revision string
	rev, err := s.getLatestGistCommit(ctx, gist, principal)
	if err == nil && rev != nil {
		revision = rev.GetVersion()
	}

	return gist, revision, err
}

func (s *GistStore) patchGist(ctx context.Context, principal *Principal, key *StoreKey, dr *DataRequest) (*gists.Gist, string, error) {
	if principal == nil {
		return nil, "", fmt.Errorf("authentication required to patch a gist")
	}

	gist, err := updateGist(dr, &gists.Gist{
		ID: github.Ptr(key.Id),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to update gist: %w", err)
	}

	client := s.getClient(ctx, principal)
	// If the user doesn't own the Gist, the API behaves as if it doesn't exist, replying with a 404
	result, resp, err := client.Edit(context.Background(), key.Id, gist)
	if resp != nil {
		if ok, err := checkResponseError(resp); !ok {
			return nil, "", err
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to patch gist: %w", err)
	}

	log.Debugf("Patch gist response: %v", resp)
	gj, _ := json.Marshal(result)
	log.Debugf("Gist patched: %s", gj)

	var revision string
	rev, err := s.getLatestGistCommit(ctx, result, principal)
	if err == nil && rev != nil {
		revision = rev.GetVersion()
	}

	return gist, revision, nil
}

func updateGist(dr *DataRequest, gist *gists.Gist) (*gists.Gist, error) {
	if gist.Files == nil {
		gist.Files = make(map[gists.GistFilename]gists.GistFile)
	}

	for k, v := range dr.RegoModules {
		if content, ok := v.(string); ok {
			gist.Files[gists.GistFilename(k)] = gists.GistFile{
				Content: github.Ptr(content),
			}
		} else {
			log.Debugf("Skipping non-string module content for key: %s", k)
		}
	}

	if dr.Input != nil {
		inputContent, err := json.Marshal(dr.Input)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal input: %w", err)
		}
		gist.Files["input.json"] = gists.GistFile{
			Content: github.Ptr(string(inputContent)),
		}
	} else {
		log.Debug("No input provided for gist creation")
		gist.Files["input.json"] = gists.GistFile{
			Content: github.Ptr("{}"), // Provide an empty input document if none is specified
		}
	}

	if dr.Data != nil {
		dataContent, err := json.Marshal(dr.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data: %w", err)
		}
		gist.Files["data.json"] = gists.GistFile{
			Content: github.Ptr(string(dataContent)),
		}
	} else {
		log.Debug("No data provided for gist creation")
		gist.Files["data.json"] = gists.GistFile{
			Content: github.Ptr("{}"), // Provide an empty data document if none is specified
		}
	}

	if meta := metadataFromDataRequest(dr); meta != nil {
		gist.Files["rego_playground_metadata.json"] = gists.GistFile{
			Content: github.Ptr(meta.toJSON()),
		}
	} else {
		log.Debug("No metadata provided for gist creation")
		gist.Files["rego_playground_metadata.json"] = gists.GistFile{
			Content: github.Ptr("{}"), // Provide an empty metadata document if none is specified
		}
	}

	return gist, nil
}

func (s *GistStore) getGistCommits(ctx context.Context, gist *gists.Gist, principal *Principal) ([]*gists.GistCommit, error) {
	if history := gist.GetHistory(); len(history) > 0 {
		log.Debugf("Gist %s has history, returning cached commits", gist.GetID())
		return history, nil
	}

	client := s.getClient(ctx, principal)

	commits, resp, err := client.ListCommits(ctx, gist.GetID(), nil)

	if resp != nil {
		if ok, err := checkResponseError(resp); !ok {
			return nil, err
		}
	}

	if err != nil {
		log.Errorf("Failed to list commits for gist %s: %v", gist.GetID(), err)
		return nil, err
	}

	log.Debugf("List commits response: %v", resp)
	return commits, nil
}

func (s *GistStore) getLatestGistCommit(ctx context.Context, gist *gists.Gist, principal *Principal) (*gists.GistCommit, error) {
	commits, err := s.getGistCommits(ctx, gist, principal)
	if err != nil {
		return nil, fmt.Errorf("failed to get gist commits: %w", err)
	}

	if len(commits) == 0 {
		log.Debugf("No commits found for gist %s", gist.GetID())
		return nil, nil
	}

	log.Debugf("Latest commit for gist %s: %s", gist.GetID(), commits[0].GetVersion())

	// Return the latest commit (first in the list)
	return commits[0], nil
}

func (s *GistStore) getClient(ctx context.Context, principal *Principal) *gists.GistsService {
	var c *github.Client
	if principal != nil {
		c = principal.Client(ctx)
	} else {
		c = github.NewClient(nil)
	}

	if s.baseUrl != nil {
		log.Debugf("Using GistStore base URL: %s", s.baseUrl)
		c.BaseURL = s.baseUrl
	}

	return gists.NewGistsService(c)
}
