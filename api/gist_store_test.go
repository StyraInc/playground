// Copyright 2025 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-github/v73/github"
	"github.com/open-policy-agent/opa/v1/util"
	gists "github.com/open-policy-agent/rego-playground/internal/github"
	"golang.org/x/oauth2"
)

func TestGistStore_Get(t *testing.T) {
	cases := []struct {
		note      string
		key       *StoreKey
		principal *Principal
		gist      *gists.Gist
		commits   []*gists.GistCommit
		status    int
		headers   map[string]string
		expResult *DataRequest
		expErr    string
	}{
		{
			note:   "no principal",
			key:    &StoreKey{Id: "foo"},
			expErr: "unauthorized: authentication required to get a gist",
		},
		{
			note:      "forbidden",
			principal: &Principal{},
			key:       &StoreKey{Id: "foo"},
			status:    403,
			expErr:    "unauthorized: forbidden",
		},
		{
			note:      "get key, found",
			key:       &StoreKey{Id: "foo"},
			principal: &Principal{},
			status:    200,
			gist:      makeGist("foo"),
			commits:   makeGistCommits("bar"),
			expResult: makeDataRequest(
				"policy.rego",
				`package example`,
				`{}`,
				`{}`,
				1,
				"bar"),
		},
		{
			// FIXME: Should this be an error?
			note:      "get key, found (missing revision)",
			key:       &StoreKey{Id: "foo"},
			principal: &Principal{},
			status:    200,
			gist:      makeGist("foo"),
			expResult: makeDataRequest(
				"policy.rego",
				`package example`,
				`{}`,
				`{}`,
				1,
				""),
		},
		{
			note:      "get key, not found",
			key:       &StoreKey{Id: "foo"},
			status:    404,
			principal: &Principal{},
		},
		{
			note:      "get key+revision, found",
			key:       &StoreKey{Id: "foo", Revision: "bar"},
			principal: &Principal{},
			status:    200,
			gist:      makeGist("foo"),
			expResult: makeDataRequest(
				"policy.rego",
				`package example`,
				`{}`,
				`{}`,
				1,
				"bar"),
		},
		{
			note:      "get key+revision, not found",
			key:       &StoreKey{Id: "foo", Revision: "bar"},
			status:    404,
			principal: &Principal{},
		},
		{
			note:      "no files on gist",
			key:       &StoreKey{Id: "foo"},
			principal: &Principal{},
			status:    200,
			gist: &gists.Gist{
				ID: github.Ptr("foo"),
			},
			expErr: "gist <foo> found but has no files",
		},
		{
			note:      "rate limit exceeded",
			key:       &StoreKey{Id: "foo"},
			principal: &Principal{},
			status:    403,
			headers: map[string]string{
				"X-RateLimit-Remaining": "0",
			},
			expErr: "rate-limited: rate limit exceeded",
		},
		{
			note:      "forbidden, but not rate-limited",
			key:       &StoreKey{Id: "foo"},
			principal: &Principal{},
			status:    403,
			headers: map[string]string{
				"X-RateLimit-Remaining": "5",
			},
			expErr: "unauthorized: forbidden",
		},
		{
			note:      "422 (spammed)",
			key:       &StoreKey{Id: "foo"},
			principal: &Principal{},
			status:    422,
			expErr:    "rate-limited: spammed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Printf("Request: %s %s\n", r.Method, r.URL.Path)

				if strings.HasPrefix(r.URL.Path, fmt.Sprintf("/gists/%s/commits", tc.key.Id)) {
					bs, err := json.Marshal(tc.commits)
					if err != nil {
						t.Fatalf("Failed to marshal gist commits: %v", err)
					}
					_, _ = w.Write(bs)
					w.WriteHeader(http.StatusOK)
				} else if strings.HasPrefix(r.URL.Path, "/gists") {
					if tc.key.Revision != "" {
						if path, expPath := r.URL.Path, fmt.Sprintf("/gists/%s/%s", tc.key.Id, tc.key.Revision); path != expPath {
							t.Errorf("Unexpected request path: got %s, want %s", path, expPath)
						}
					} else {
						if path, expPath := r.URL.Path, fmt.Sprintf("/gists/%s", tc.key.Id); path != expPath {
							t.Errorf("Unexpected request path: got %s, want %s", path, expPath)
						}
					}

					if tc.gist != nil {
						bs, err := json.Marshal(tc.gist)
						if err != nil {
							t.Fatalf("Failed to marshal gist: %v", err)
						}
						_, _ = w.Write(bs)
					}

					for k, v := range tc.headers {
						w.Header().Set(k, v)
					}

					w.WriteHeader(tc.status)
				} else {
					t.Errorf("Unexpected request path: %s", r.URL.Path)
				}
			}))
			defer srv.Close()

			baseUrl, _ := url.Parse(srv.URL + "/")

			store := NewGistStore(GistStoreBaseUrl(baseUrl))

			if tc.principal != nil {
				tc.principal.oauthConfig = &oauth2.Config{
					ClientID:     "foo",
					ClientSecret: "bar",
					RedirectURL:  "baz",
					Endpoint: oauth2.Endpoint{
						AuthURL:  srv.URL + "/oauth/authorize",
						TokenURL: srv.URL + "/oauth/token",
					},
					Scopes: []string{"gist"},
				}
				tc.principal.accessToken = &oauth2.Token{
					AccessToken: "foobar",
				}
			}

			result, found, err := store.Get(tc.key, tc.principal)

			if tc.expErr != "" {
				if err == nil {
					t.Errorf("Get did not return expected error: %v", tc.expErr)
				}
				if err.Error() != tc.expErr {
					t.Errorf("Get returned unexpected error: got %v, want %v", err.Error(), tc.expErr)
				}
			} else {
				if err != nil {
					t.Errorf("Get returned an unexpected error: %v", err)
				}
			}

			if tc.expResult != nil {
				if !found {
					t.Errorf("Get did not find expected data for key: %s", tc.key.Id)
				}
				if !reflect.DeepEqual(result, *tc.expResult) {
					t.Errorf("Get returned unexpected data, got:\n\n%s\n\nwant:\n\n%s", toJson(t, result), toJson(t, *tc.expResult))
				}
			} else {
				if found {
					t.Errorf("Get found unexpected data: %s", tc.key.Id)
				}
			}
		})
	}
}

func TestGistStore_Put(t *testing.T) {
	defaultId := "some_id"
	defaultRev := "some_rev"

	type serve struct {
		expId   string
		expGist *gists.Gist
		gist    *gists.Gist
		status  int
		headers map[string]string
	}

	cases := []struct {
		note        string
		key         *StoreKey
		principal   *Principal
		dataRequest DataRequest
		createGist  *serve
		updateGist  *serve
		expKey      *StoreKey
		expErr      string
	}{
		{
			note:   "no principal",
			expErr: "unauthorized: authentication required to upsert a gist",
		},

		{
			note:      "forbidden (create gist)",
			principal: &Principal{},
			createGist: &serve{
				status: 403,
			},
			expErr: "unauthorized: forbidden",
		},
		{
			note:      "forbidden (create gist, on patch README)",
			principal: &Principal{},
			createGist: &serve{
				status: 201,
			},
			updateGist: &serve{
				status: 403,
			},
			// We ignore failed updates to the README, as long as the gist was created successfully.
			expKey: &StoreKey{Id: defaultId, Revision: defaultRev, KeyType: KeyTypeGist},
		},
		{
			note:      "forbidden (update gist)",
			principal: &Principal{},
			key:       &StoreKey{Id: "foo"},
			updateGist: &serve{
				status: 403,
			},
			expErr: "unauthorized: forbidden",
		},

		{
			note:      "rate-limited (create gist)",
			principal: &Principal{},
			createGist: &serve{
				status: 403,
				headers: map[string]string{
					"X-RateLimit-Remaining": "0",
				},
			},
			expErr: "rate-limited: rate limit exceeded",
		},
		{
			note:      "rate-limited (create gist, on patch README)",
			principal: &Principal{},
			createGist: &serve{
				status: 201,
			},
			updateGist: &serve{
				status: 403,
				headers: map[string]string{
					"X-RateLimit-Remaining": "0",
				},
			},
			// We ignore failed updates to the README, as long as the gist was created successfully.
			expKey: &StoreKey{Id: defaultId, Revision: defaultRev, KeyType: KeyTypeGist},
		},
		{
			note:      "rate-limited (update gist)",
			principal: &Principal{},
			key:       &StoreKey{Id: "foo"},
			updateGist: &serve{
				status: 403,
				headers: map[string]string{
					"X-RateLimit-Remaining": "0",
				},
			},
			expErr: "rate-limited: rate limit exceeded",
		},

		{
			note:      "spammed (create gist)",
			principal: &Principal{},
			createGist: &serve{
				status: 422,
			},
			expErr: "rate-limited: spammed",
		},
		{
			note:      "spammed (create gist, on patch README)",
			principal: &Principal{},
			createGist: &serve{
				status: 201,
			},
			updateGist: &serve{
				status: 422,
			},
			// We ignore failed updates to the README, as long as the gist was created successfully.
			expKey: &StoreKey{Id: defaultId, Revision: defaultRev, KeyType: KeyTypeGist},
		},
		{
			note:      "spammed (update gist)",
			principal: &Principal{},
			key:       &StoreKey{Id: "foo"},
			updateGist: &serve{
				status: 422,
			},
			expErr: "rate-limited: spammed",
		},

		{
			note:      "create gist",
			principal: &Principal{},
			dataRequest: *makeDataRequest(
				"policy.rego",
				`package example`,
				`{"foo": "bar"}`,
				`{"baz": "qux"}`,
				1,
				""),
			createGist: &serve{
				expGist: &gists.Gist{
					Files: map[gists.GistFilename]gists.GistFile{
						"policy.rego": {
							Filename: github.Ptr("policy.rego"),
							Content:  github.Ptr(`package example`),
						},
						"data.json": {
							Filename: github.Ptr("data.json"),
							Content:  github.Ptr(`{}`),
						},
						"input.json": {
							Filename: github.Ptr("input.json"),
							Content:  github.Ptr(`{}`),
						},
						"rego_playground_metadata.json": {
							Filename: github.Ptr("rego_playground_metadata.json"),
							Content:  github.Ptr(`{"rego_version":1}`),
						},
					},
				},
				status: 201,
				gist: &gists.Gist{
					ID: github.Ptr(defaultId),
					Files: map[gists.GistFilename]gists.GistFile{
						"policy.rego": {
							Filename: github.Ptr("policy.rego"),
							Content:  github.Ptr(`package example`),
						},
						"data.json": {
							Filename: github.Ptr("data.json"),
							Content:  github.Ptr(`{}`),
						},
						"input.json": {
							Filename: github.Ptr("input.json"),
							Content:  github.Ptr(`{}`),
						},
						"rego_playground_metadata.json": {
							Filename: github.Ptr("rego_playground_metadata.json"),
							Content:  github.Ptr(`{"rego_version":1}`),
						},
					},
					History: makeGistCommits(defaultRev),
				},
			},
			// The patch to update the REAMDE
			updateGist: &serve{
				status: 200,
				gist: &gists.Gist{
					ID: github.Ptr(defaultId),
					Files: map[gists.GistFilename]gists.GistFile{
						"policy.rego": {
							Filename: github.Ptr("policy.rego"),
							Content:  github.Ptr(`package example`),
						},
						"data.json": {
							Filename: github.Ptr("data.json"),
							Content:  github.Ptr(`{}`),
						},
						"input.json": {
							Filename: github.Ptr("input.json"),
							Content:  github.Ptr(`{}`),
						},
						"rego_playground_metadata.json": {
							Filename: github.Ptr("rego_playground_metadata.json"),
							Content:  github.Ptr(`{"rego_version":1}`),
						},
					},
					History: makeGistCommits("readme", defaultRev),
				},
			},
			expKey: &StoreKey{Id: defaultId, Revision: "readme", KeyType: KeyTypeGist},
		},

		{
			note:      "update gist",
			principal: &Principal{},
			key:       &StoreKey{Id: "foo"},
			dataRequest: *makeDataRequest(
				"policy.rego",
				`package example`,
				`{"foo": "bar"}`,
				`{"baz": "qux"}`,
				1,
				""),
			updateGist: &serve{
				expGist: &gists.Gist{
					ID: github.Ptr("foo"),
					Files: map[gists.GistFilename]gists.GistFile{
						"policy.rego": {
							Content: github.Ptr(`package example`),
						},
						"data.json": {
							Content: github.Ptr(`{"foo":"bar"}`),
						},
						"input.json": {
							Content: github.Ptr(`{"baz":"qux"}`),
						},
						"rego_playground_metadata.json": {
							Content: github.Ptr(`{"coverage":false,"rego_version":1}`),
						},
					},
				},
				status: 200,
				gist: &gists.Gist{
					ID: github.Ptr("foo"),
					Files: map[gists.GistFilename]gists.GistFile{
						"policy.rego": {
							Filename: github.Ptr("policy.rego"),
							Content:  github.Ptr(`package example`),
						},
						"data.json": {
							Filename: github.Ptr("data.json"),
							Content:  github.Ptr(`{"foo": "bar"}`),
						},
						"input.json": {
							Filename: github.Ptr("input.json"),
							Content:  github.Ptr(`{"baz": "qux"}`),
						},
						"rego_playground_metadata.json": {
							Filename: github.Ptr("rego_playground_metadata.json"),
							Content:  github.Ptr(`{"rego_version":1}`),
						},
					},
					History: makeGistCommits("bar"),
				},
			},
			expKey: &StoreKey{Id: "foo", Revision: "bar", KeyType: KeyTypeGist},
		},

		{
			// NOTE: The Gist API will return a 404 when the active user isn't the owner of the Gist, as if it didn't exist.
			// So this test covers both the case when a Gist can't be found or has been deleted, and the case where
			// someone else owns the Gist and we're not allowed to update it.
			note:      "update gist (not found)",
			principal: &Principal{},
			key:       &StoreKey{Id: "foo"},
			dataRequest: *makeDataRequest(
				"policy.rego",
				`package example`,
				`{"foo": "bar"}`,
				`{"baz": "qux"}`,
				1,
				""),
			// if Gist isn't found or it belongs to another user ...
			updateGist: &serve{
				status: 404,
			},
			// ... then we expect a new Gist to be created
			createGist: &serve{
				expGist: &gists.Gist{
					ID: github.Ptr("foo"),
					Files: map[gists.GistFilename]gists.GistFile{
						"policy.rego": {
							Content: github.Ptr(`package example`),
						},
						"data.json": {
							Content: github.Ptr(`{"foo":"bar"}`),
						},
						"input.json": {
							Content: github.Ptr(`{"baz":"qux"}`),
						},
						"rego_playground_metadata.json": {
							Content: github.Ptr(`{"trace":false,"coverage":false,"strict":false,"read_only":false,"rego_version":1}`),
						},
					},
				},
				status: 200,
				gist: &gists.Gist{
					ID: github.Ptr("foo"),
					Files: map[gists.GistFilename]gists.GistFile{
						"policy.rego": {
							Filename: github.Ptr("policy.rego"),
							Content:  github.Ptr(`package example`),
						},
						"data.json": {
							Filename: github.Ptr("data.json"),
							Content:  github.Ptr(`{"foo": "bar"}`),
						},
						"input.json": {
							Filename: github.Ptr("input.json"),
							Content:  github.Ptr(`{"baz": "qux"}`),
						},
						"rego_playground_metadata.json": {
							Filename: github.Ptr("rego_playground_metadata.json"),
							Content:  github.Ptr(`{"rego_version":1}`),
						},
					},
					History: makeGistCommits("bar"),
				},
			},
			expKey: &StoreKey{Id: "foo", Revision: "bar", KeyType: KeyTypeGist},
		},
	}

	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Printf("Request: %s %s\n", r.Method, r.URL.Path)

				var newGist *gists.Gist
				if r.ContentLength > 0 {
					newGist = &gists.Gist{}
					if err := json.NewDecoder(r.Body).Decode(newGist); err != nil {
						t.Fatalf("Failed to decode gist from request body: %v", err)
					}
				}

				if tc.createGist != nil && strings.HasPrefix(r.URL.Path, "/gists") && r.Method == http.MethodPost {
					if g := tc.createGist.gist; g != nil {
						bs, err := json.Marshal(g)
						if err != nil {
							t.Fatalf("Failed to marshal gist: %v", err)
						}
						_, _ = w.Write(bs)
					} else if tc.createGist.status == 201 {
						newGist.ID = github.Ptr(defaultId)
						newGist.History = makeGistCommits(defaultRev)
						bs, err := json.Marshal(newGist)
						if err != nil {
							t.Fatalf("Failed to marshal gist: %v", err)
						}
						_, _ = w.Write(bs)
					}

					for k, v := range tc.createGist.headers {
						w.Header().Set(k, v)
					}

					if s := tc.createGist.status; s != 0 {
						w.WriteHeader(s)
					}
				} else if tc.updateGist != nil && strings.HasPrefix(r.URL.Path, "/gists/") && r.Method == http.MethodPatch {
					if tc.updateGist.expGist != nil {
						if newGist == nil {
							t.Fatalf("Expected a gist in request body, but got nil")
						}
						if !reflect.DeepEqual(*newGist, *tc.updateGist.expGist) {
							t.Errorf("Get returned unexpected data, got:\n\n%s\n\nwant:\n\n%s", toJson(t, newGist), toJson(t, *tc.updateGist.expGist))
						}
					}

					if g := tc.updateGist.gist; g != nil {
						bs, err := json.Marshal(g)
						if err != nil {
							t.Fatalf("Failed to marshal gist: %v", err)
						}
						_, _ = w.Write(bs)
					}

					for k, v := range tc.updateGist.headers {
						w.Header().Set(k, v)
					}

					if s := tc.updateGist.status; s != 0 {
						w.WriteHeader(s)
					}
				} else {
					t.Errorf("Unexpected request path: %s", r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()

			baseUrl, _ := url.Parse(srv.URL + "/")

			store := NewGistStore(GistStoreBaseUrl(baseUrl))

			if tc.principal != nil {
				tc.principal.oauthConfig = &oauth2.Config{
					ClientID:     "foo",
					ClientSecret: "bar",
					RedirectURL:  "baz",
					Endpoint: oauth2.Endpoint{
						AuthURL:  srv.URL + "/oauth/authorize",
						TokenURL: srv.URL + "/oauth/token",
					},
					Scopes: []string{"gist"},
				}
				tc.principal.accessToken = &oauth2.Token{
					AccessToken: "foobar",
				}
			}

			//result, found, err := store.Get(tc.key, tc.principal)
			key, err := store.Put(tc.key, tc.dataRequest, tc.principal)

			if tc.expErr != "" {
				if err == nil {
					t.Errorf("Get did not return expected error: %v", tc.expErr)
				}
				if err.Error() != tc.expErr {
					t.Errorf("Get returned unexpected error: got %v, want %v", err.Error(), tc.expErr)
				}
			} else {
				if err != nil {
					t.Errorf("Get returned an unexpected error: %v", err)
				}

				if key == nil {
					t.Errorf("Put did not return a key")
				}
				if tc.expKey != nil && !reflect.DeepEqual(key, tc.expKey) {
					t.Errorf("Put returned unexpected key, got: %s, want: %s", key.Id, tc.expKey.Id)
				}
			}
		})
	}
}

func toJson(t *testing.T, v any) string {
	t.Helper()
	bs, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal value: %v", err)
	}
	return string(bs)
}

func makeGist(id string) *gists.Gist {
	g := gists.Gist{
		ID: github.Ptr(id),
		Files: map[gists.GistFilename]gists.GistFile{
			"data.json": {
				Filename: github.Ptr("data.json"),
				Content:  github.Ptr(`{}`),
			},
			"input.json": {
				Filename: github.Ptr("input.json"),
				Content:  github.Ptr(`{}`),
			},
			"policy.rego": {
				Filename: github.Ptr("policy.rego"),
				Content:  github.Ptr(`package example`),
			},
			"rego_playground_metadata.json": {
				Filename: github.Ptr("rego_playground_metadata.json"),
				Content:  github.Ptr(`{"rego_version":1}`),
			},
		},
	}

	return &g
}

func makeGistCommits(revisions ...string) []*gists.GistCommit {
	commits := make([]*gists.GistCommit, len(revisions))
	for i, rev := range revisions {
		commits[i] = &gists.GistCommit{
			Version: github.Ptr(rev),
		}
	}
	return commits
}

func makeDataRequest(moduleName string, module string, data string, input string, regoVersion int, eTag string) *DataRequest {
	out := DataRequest{}

	if module != "" {
		out.RegoModules = map[string]interface{}{
			moduleName: module,
		}
	} else {
		out.RegoModules = map[string]interface{}{
			"play/play.rego": "package play",
		}
	}

	if input != "" {
		util.UnmarshalJSON([]byte(input), &out.Input)
	}

	if data != "" {
		util.UnmarshalJSON([]byte(data), &out.Data)
	}

	out.RegoVersion = &regoVersion

	if eTag != "" {
		out.Etag = eTag
	}

	return &out
}
