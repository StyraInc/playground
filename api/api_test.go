package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/mattbaird/jsonpatch"
	"github.com/open-policy-agent/opa/bundle"

	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/util"
)

// TODO: Test store fallback
// TODO: Test v2 API

func TestApiEvalPrintOutput(t *testing.T) {
	dr := makeDR(`
		package test

		p { print("hello", "world") }`, `p`, `{}`, 0)

	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.handleQuery(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res struct {
		Result rego.ResultSet `json:"result"`
		Output string         `json:"output"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	exp := "play.rego:4: hello world\n"

	if exp != res.Output {
		t.Fatalf("unexpected print output: %q (expected %q)", res.Output, exp)
	}
}

func TestApiEvalLargeNumbers(t *testing.T) {
	dr := makeDR("package test\ndefault allow = false\nallow { input.foo = 9007199254740993 }", `allow`, `{"foo": 9007199254740993}`, 0)
	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.doHandleQuery(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res struct {
		Result rego.ResultSet `json:"result"`
		Output string         `json:"output"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res.Result[0].Expressions[0].Value, true) {
		t.Fatalf("unexpected result for policy containing large numbers: %v", res.Result[0].Expressions[0].Value)
	}
}

func TestApiEvalStrictBuiltInErrors(t *testing.T) {
	dr := makeDR(`
package play

import future.keywords.if

allow if concat(",", input.non_collection) == "foobar"

allow if 2 / 0 == 2
`, `allow`, `{}`, 0)

	dr.BuiltInErrorsStrict = true

	input := interface{}(map[string]interface{}{"non_collection": "1"})
	dr.Input = &input

	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.handleQuery(w, r)

	if w.Code != 400 {
		t.Fatalf("expected 500 response but got: %v", w.Code)
	}

	var resErr apiError
	err := json.Unmarshal(w.Body.Bytes(), &resErr)
	if err != nil {
		t.Fatalf("failed to unmarshal error response: %s", err)
	}

	if exp, act := "internal_error", resErr.Code; exp != act {
		t.Errorf("unexpected error code, got %s, expected: %s", act, exp)
	}

	exp := "play.rego:8: eval_builtin_error: div: divide by zero"
	if exp != resErr.Message {
		t.Errorf("unexpected error message, got %s, expected: %s", resErr.Message, exp)
	}

	errs, ok := resErr.Error.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected error type %T", resErr.Error)
	}

	b, err := json.MarshalIndent(errs, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal errors as json: %s", err)
	}

	exp = `{
  "code": "eval_builtin_error",
  "location": {
    "col": 10,
    "file": "play.rego",
    "row": 8
  },
  "message": "div: divide by zero"
}`
	if act := string(b); exp != act {
		t.Errorf("unexpected error data, expected: %s, got: %s", exp, act)
	}
}

func TestApiEvalAllBuiltInErrors(t *testing.T) {
	dr := makeDR(`
package play

import future.keywords.if

allow if concat(",", input.non_collection) == "foobar"

allow if 2 / 0 == 2
`, `allow`, `{}`, 0)

	dr.BuiltInErrorsAll = true

	input := interface{}(map[string]interface{}{"non_collection": "1"})
	dr.Input = &input

	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body))
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.handleQuery(w, r)

	if w.Code != 400 {
		t.Fatalf("expected 400 response but got: %v", w.Code)
	}

	var resErr apiError
	err := json.Unmarshal(w.Body.Bytes(), &resErr)
	if err != nil {
		t.Fatalf("failed to unmarshal error response: %s", err)
	}

	if exp, act := "internal_error", resErr.Code; exp != act {
		t.Errorf("unexpected error code, got %s, expected: %s", act, exp)
	}

	exp := `2 errors occurred:
play.rego:8: eval_builtin_error: div: divide by zero
play.rego:6: eval_type_error: concat: operand 2 must be one of {set, array} but got string`
	if exp != resErr.Message {
		t.Errorf("unexpected error message, got %s, expected: %s", resErr.Message, exp)
	}

	errs, ok := resErr.Error.([]interface{})
	if !ok {
		t.Fatalf("unexpected error type %T", resErr.Error)
	}

	b, err := json.MarshalIndent(errs, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal errors as json: %s", err)
	}

	exp = `[
  {
    "code": "eval_builtin_error",
    "location": {
      "col": 10,
      "file": "play.rego",
      "row": 8
    },
    "message": "div: divide by zero"
  },
  {
    "code": "eval_type_error",
    "location": {
      "col": 10,
      "file": "play.rego",
      "row": 6
    },
    "message": "concat: operand 2 must be one of {set, array} but got string"
  }
]`
	if act := string(b); exp != act {
		t.Errorf("unexpected error data, expected: %s, got: %s", exp, act)
	}
}

func TestApiEvalOPARuntimeInPolicy(t *testing.T) {
	dr := makeDR(`
		package test
		p = opa.runtime()`, `p`, `{}`, 0)
	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.handleQuery(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res struct {
		Result rego.ResultSet `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	exp := map[string]interface{}{
		"message": "The Rego Playground does not provide OPA runtime information during policy execution.",
	}

	if !reflect.DeepEqual(res.Result[0].Expressions[0].Value, exp) {
		t.Fatalf("unexpected result for policy containing opa.runtime() call: %v", res.Result[0].Expressions[0].Value)
	}
}

func TestApiEvalOPARuntimeInQuery(t *testing.T) {
	dr := makeDR(`
		package test
		p = true`, `p; x := opa.runtime()`, `{}`, 0)
	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.handleQuery(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res struct {
		Result rego.ResultSet `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	exp := map[string]interface{}{
		"message": "The Rego Playground does not provide OPA runtime information during policy execution.",
	}

	if !reflect.DeepEqual(res.Result[0].Bindings["x"], exp) {
		t.Fatalf("unexpected result for policy containing opa.runtime() call: %v", res.Result[0].Bindings["x"])
	}
}

func TestApiEvalUnsafeBuiltins(t *testing.T) {
	tests := []struct {
		note string
		dr   DataRequest
	}{
		{
			note: "in policy",
			dr: makeDR(`
package test
p {
	http.send({"method": "get", "url": "https://httpbin.org"})
}`,
				"p", "{}", 0),
		},
		{
			note: "in policy, 'with'",
			dr: makeDR(`
package test
p {
	is_object({"method": "get", "url": "https://httpbin.org"}) with is_object as http.send
}`,
				"p", "{}", 0),
		},
		{
			note: "in query",
			dr: makeDR(`
package test
p = true
`,
				`p; http.send({"method": "get", "url": "https://httpbin.org"})`, "{}", 0),
		},
		{
			note: "in query, 'with'",
			dr: makeDR(`
package test
p = true
`,
				`p; is_object({"method": "get", "url": "https://httpbin.org"}) with is_object as http.send`, "{}", 0),
		},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {
			body, err := json.Marshal(tc.dr)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
			s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

			s.handleQuery(w, r)

			if w.Code != 400 {
				t.Fatalf("expected 400 response but got: %v", w.Code)
			}
		})
	}
}

func TestApiEvalWithCoverage(t *testing.T) {
	dr := makeDR("package test\ndefault allow = false\nallow { input.foo = 1 }", `allow`, `{"foo": 1}`, 0)
	dr.Coverage = true
	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.doHandleQuery(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res DataResponse
	json.Unmarshal(w.Body.Bytes(), &res)

	if res.Coverage == nil {
		t.Fatal("expected coverage to be set")
	}
}

func TestApiLint(t *testing.T) {
	lr := LintRequest{
		RegoModule: "package test\nimport rego.v1\nthing = 1\n",
	}

	body, _ := json.Marshal(lr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/lint", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.handleLint(w, r)

	if w.Code != 200 {
		bs, err := io.ReadAll(w.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %s", err)
		}

		t.Fatalf("expected 200 response but got: %v, body: %s", w.Code, string(bs))
	}

	var res LintResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	if err != nil {
		t.Fatalf("failed to unmarshal response: %s", err)
	}

	if res.Report == nil {
		t.Fatal("expected lint report to be set")
	}

	if len(res.Report.Violations) != 2 {
		t.Fatal("expected 2 violations in response but got:", len(res.Report.Violations))
	}

	expectedViolations := []string{"use-assignment-operator", "opa-fmt"}
	for _, expected := range expectedViolations {
		found := false
		for _, violation := range res.Report.Violations {
			if violation.Title == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected violation %s but not found", expected)
		}
	}
}

func TestApiLintErrors(t *testing.T) {
	lr := LintRequest{
		RegoModule: "package test\npackage foobar\n",
	}

	body, _ := json.Marshal(lr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/lint", bytes.NewReader(body)) // These values don't matter
	s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	s.handleLint(w, r)

	if w.Code != 200 {
		bs, err := io.ReadAll(w.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %s", err)
		}

		t.Fatalf("expected 200 response but got: %v, body: %s", w.Code, string(bs))
	}

	var res LintResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	if err != nil {
		t.Fatalf("failed to unmarshal response: %s", err)
	}

	if len(res.Errors) != 1 {
		t.Fatal("expected 1 errors in response but got:", len(res.Errors))
	}

	astError := res.Errors[0]
	if exp, got := "rego_parse_error", astError.Code; exp != got {
		t.Fatalf("expected error code %v but got: %v", exp, got)
	}
}

func TestApiEvalWithRegoVersion(t *testing.T) {
	regoVersion0 := 0
	regoVersion1 := 1

	tests := []struct {
		note        string
		module      string
		regoVersion *int
		query       string
		expCode     int
	}{
		{
			note: "query containing future keywords, rego.v1 imported, no rego version",
			module: `package test
import rego.v1

p if {
	some x in ["a", "b", "c"]
}`,
			regoVersion: nil,
			query:       "p",
			expCode:     200,
		},
		{
			note: "query containing future keywords, rego.v1 imported, rego version 0",
			module: `package test
import rego.v1

p if {
	some x in ["a", "b", "c"]
}`,
			regoVersion: &regoVersion0,
			query:       "p",
			expCode:     200,
		},
		{
			note: "query containing future keywords, rego.v1 imported, rego version 1",
			module: `package test
import rego.v1

p if {
	some x in ["a", "b", "c"]
}`,
			regoVersion: &regoVersion1,
			query:       "p",
			expCode:     200,
		},
		{
			note: "query containing future keywords, no rego.v1 imported, no rego version",
			module: `package test

p if {
	some x in ["a", "b", "c"]
}`,
			regoVersion: nil,
			query:       "p",
			expCode:     200,
		},
		{
			note: "query containing future keywords, no rego.v1 imported, rego version 0",
			module: `package test

p if {
	some x in ["a", "b", "c"]
}`,
			regoVersion: &regoVersion0,
			query:       "p",
			expCode:     400,
		},
		{
			note: "query containing future keywords, no rego.v1 imported, rego version 1",
			module: `package test

p if {
	some x in ["a", "b", "c"]
}`,
			regoVersion: &regoVersion1,
			query:       "p",
			expCode:     200,
		},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {
			var dr DataRequest
			if tc.regoVersion != nil {
				dr = makeDR(tc.module, tc.query, "", *tc.regoVersion)
			} else {
				dr = makeDR(tc.module, tc.query, "", 0)
				dr.RegoVersion = nil
			}
			body, _ := json.Marshal(dr)
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
			s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

			s.handleQuery(w, r)

			if tc.expCode != w.Code {
				t.Errorf("unexpected error code, got %v, expected: %v", w.Code, tc.expCode)
			}
		})
	}
}

func TestApiEvalWithQuery(t *testing.T) {
	tests := []struct {
		note   string
		module string
		query  string
		input  string
		exp    []rego.Vars
	}{
		{
			note: "simple query",
			module: `package test
p { x := 1 }`,
			query: `x := 2`,
			exp: []rego.Vars{
				{"x": float64(2)},
			},
		},
		{
			note: "query containing future keywords, future.keywords imported",
			module: `package test
import future.keywords

p {
	some x in ["a", "b", "c"]
}`,
			query: `some x in ["a", "b", "c"]`,
			exp: []rego.Vars{
				{"x": "a"},
				{"x": "b"},
				{"x": "c"},
			},
		},
		{
			note: "query containing future keywords, rego.v1 imported",
			module: `package test
import rego.v1

p if {
	some x in ["a", "b", "c"]
}`,
			query: `some x in ["a", "b", "c"]`,
			exp: []rego.Vars{
				{"x": "a"},
				{"x": "b"},
				{"x": "c"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {
			dr := makeDR(tc.module, tc.query, tc.input, 0)
			dr.Strict = true
			body, _ := json.Marshal(dr)
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
			s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

			s.doHandleQuery(w, r)

			if w.Code != 200 {
				t.Fatalf("expected 200 response but got: %v", w.Code)
			}

			var res struct {
				Result rego.ResultSet `json:"result"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}

			var bindings []rego.Vars
			for _, r := range res.Result {
				bindings = append(bindings, r.Bindings)
			}

			if !reflect.DeepEqual(bindings, tc.exp) {
				t.Fatalf("expected result:\n\n%v\n\ngot:\n\n%v", tc.exp, bindings)
			}
		})
	}
}

func TestApiHandleRetrieveFromStore(t *testing.T) {
	store := NewMemoryDataRequestStore()
	s := NewAPIService("", store, nil, "./", "", "", "")
	key := StoreKey{Id: "foo"}

	dr := makeDR("package test\ndefault allow = false\nallow { input.foo = 1\nx := 1 }", `allow`, `{"foo": 1}`, 0)
	data := map[string]interface{}{"foo": "bar"}
	dr.Data = util.Reference(data)

	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/data", bytes.NewReader(body))

	dr.Etag = key.Id
	_, err := store.Put(&key, dr, nil)
	if err != nil {
		t.Fatal(err)
	}

	u := url.URL{
		Scheme: "https",
		Host:   "example.com",
	}

	q := u.Query()
	q.Set("coverage", "true")
	q.Set("evaluate", "true")
	u.RawQuery = q.Encode()

	s.doHandleRetrieveFromStore(r.Context(), w, &u, &key, nil)

	if w.Code != 400 {
		t.Fatalf("expected 400 response but got: %v", w.Code)
	}

	var resErr apiError
	json.Unmarshal(w.Body.Bytes(), &resErr)

	if resErr.Code != apiCodeParseError {
		t.Fatalf("expected error code %v but got: %v", apiCodeParseError, resErr.Code)
	}

	// strict-mode disabled
	q.Set("strict", "false")
	u.RawQuery = q.Encode()

	w = httptest.NewRecorder()
	s.doHandleRetrieveFromStore(r.Context(), w, &u, &key, nil)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res DataResponse
	json.Unmarshal(w.Body.Bytes(), &res)

	if res.Coverage == nil {
		t.Fatal("expected coverage to be set")
	}

	if res.Result == nil {
		t.Fatal("expected result to be set")
	}

	if res.RegoVersion == nil {
		t.Fatal("expected rego version to be set")
	}

	if *res.RegoVersion != 0 {
		t.Fatalf("unexpected rego version, got: %v", *res.RegoVersion)
	}
}

func TestApiHandleRetrieveFromStoreWithRegoVersion(t *testing.T) {
	store := NewMemoryDataRequestStore()
	s := NewAPIService("", store, nil, "./", "", "", "")
	key := StoreKey{Id: "foo"}

	dr := makeDR("package test\ndefault allow := false\nallow if { input.foo == 1\nx := 1 }", `allow`, `{"foo": 1}`, 0)
	data := map[string]interface{}{"foo": "bar"}
	dr.Data = util.Reference(data)

	regoVersion := 1
	dr.RegoVersion = &regoVersion

	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/data", bytes.NewReader(body))

	dr.Etag = key.Id
	_, err := store.Put(&key, dr, nil)
	if err != nil {
		t.Fatal(err)
	}

	u := url.URL{
		Scheme: "https",
		Host:   "example.com",
	}

	q := u.Query()
	q.Set("coverage", "true")
	q.Set("evaluate", "true")
	u.RawQuery = q.Encode()

	s.doHandleRetrieveFromStore(r.Context(), w, &u, &key, nil)

	if w.Code != 400 {
		t.Fatalf("expected 400 response but got: %v", w.Code)
	}

	var resErr apiError
	json.Unmarshal(w.Body.Bytes(), &resErr)

	if resErr.Code != apiCodeParseError {
		t.Fatalf("expected error code %v but got: %v", apiCodeParseError, resErr.Code)
	}

	// strict-mode disabled
	q.Set("strict", "false")
	u.RawQuery = q.Encode()

	w = httptest.NewRecorder()
	s.doHandleRetrieveFromStore(r.Context(), w, &u, &key, nil)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res DataResponse
	json.Unmarshal(w.Body.Bytes(), &res)

	if res.Coverage == nil {
		t.Fatal("expected coverage to be set")
	}

	if res.Result == nil {
		t.Fatal("expected result to be set")
	}

	if res.RegoVersion == nil {
		t.Fatal("expected rego version to be set")
	}

	if *res.RegoVersion != regoVersion {
		t.Fatal("unexpected rego version")
	}
}

func TestApiHandleRetrieveFromStoreLargeNumbers(t *testing.T) {
	store := NewMemoryDataRequestStore()
	s := NewAPIService("", store, nil, "./", "", "", "")
	key := StoreKey{Id: "foo"}

	dr := makeDR("package test\ndefault allow = false\nallow { input.foo = 9007199254740993 }", `allow`, `{"foo": 9007199254740993}`, 0)
	data := map[string]interface{}{"foo": 9007199254740993}
	dr.Data = util.Reference(data)

	body, _ := json.Marshal(dr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/data", bytes.NewReader(body))

	dr.Etag = key.Id
	_, err := store.Put(&key, dr, nil)
	if err != nil {
		t.Fatal(err)
	}

	u := url.URL{
		Scheme: "https",
		Host:   "example.com",
	}

	q := u.Query()
	q.Set("strict", "false")
	q.Set("coverage", "true")
	q.Set("evaluate", "true")
	u.RawQuery = q.Encode()

	s.doHandleRetrieveFromStore(r.Context(), w, &u, &key, nil)

	w = httptest.NewRecorder()
	s.doHandleRetrieveFromStore(r.Context(), w, &u, &key, nil)

	if w.Code != 200 {
		t.Fatalf("expected 200 response but got: %v", w.Code)
	}

	var res DataResponse
	util.Unmarshal(w.Body.Bytes(), &res)

	if res.Coverage == nil {
		t.Fatal("expected coverage to be set")
	}

	if res.Result == nil {
		t.Fatal("expected result to be set")
	}

	if res.Input == nil {
		t.Fatal("expected input to be set")
	}

	if res.Data == nil {
		t.Fatal("expected data to be set")
	}

	exp := int64(9007199254740993)

	ip := *res.Input
	actual, err := ip.(map[string]interface{})["foo"].(json.Number).Int64()
	if err != nil {
		t.Fatal(err)
	}

	if actual != exp {
		t.Fatalf("expected input value: %v, but got: %v", actual, exp)
	}

	dt := *res.Data
	actual, err = dt.(map[string]interface{})["foo"].(json.Number).Int64()
	if err != nil {
		t.Fatal(err)
	}

	if actual != exp {
		t.Fatalf("expected data value: %v, but got: %v", actual, exp)
	}
}

func TestPrettyResults(t *testing.T) {
	tests := []struct {
		req    DataRequest
		pretty string
	}{
		{makeDR("", `"foo" == "bar"`, `{ "foo": "bar"}`, 0), "false\n"},
		{
			makeDR("", `[x, "world"] = ["hello", y]`, "", 0),
			`+---------+---------+
|    x    |    y    |
+---------+---------+
| "hello" | "world" |
+---------+---------+
`,
		},
		{makeDR(`package example
y = 100
q {
		y == 100   # true because y refers to the global variable
}`, "", "", 0), `{
  "q": true,
  "y": 100
}
`},
		{makeDR(`package example
authorize = "allow" {
		input.user == "superuser"           # allow 'superuser' to perform any operation.
} else = "deny" {
		input.path[0] == "admin"            # disallow 'admin' operations...
		input.source_network == "external"  # from external networks.
}`, "authorize", `{"path":["admin","exec_shell"],"source_network":"external","user":"alice"}`, 0), `"deny"
`},
		{makeDR(`package example
trim_and_split(s) = x {
			t := trim(s, " ")
			x := split(t, ".")
}
`, `trim_and_split("   foo.bar ")`, "", 0), `[
  "foo",
  "bar"
]
`},
		{
			makeDR(`package play
a = [1, 2, 3, 4, 3, 4, 3, 4, 5]
b = {x | x = a[_]}`, `a; b`, "", 0),
			`+---------------------+-------------+
|          a          |      b      |
+---------------------+-------------+
| [1,2,3,4,3,4,3,4,5] | [1,2,3,4,5] |
+---------------------+-------------+
`,
		},
		{
			makeDR(`package example
sites = [
		{"name": "prod"},
		{"name": "smoke1"},
		{"name": "dev"}
]

q[name] { name := sites[_].name }`, `q[x]`, "", 0),
			`+----------+----------+
|    x     |   q[x]   |
+----------+----------+
| "dev"    | "dev"    |
| "prod"   | "prod"   |
| "smoke1" | "smoke1" |
+----------+----------+
`,
		},
	}

	for _, test := range tests {
		body, _ := json.Marshal(test.req)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
		NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "").handleQuery(w, r)
		var res DataResponse
		json.Unmarshal(w.Body.Bytes(), &res)
		if res.Pretty != test.pretty {
			t.Errorf("Eval resulted in pretty output\n\n%s\n\ninstead of\n\n%s\n\nRequest:\n\n%v", res.Pretty, test.pretty, test.req)
		}
	}
}

func TestJSONErrors(t *testing.T) {
	tests := []struct {
		req DataRequest
		err interface{}
	}{
		{makeDR(`package example
r {
	z == 100   # compiler error because z has not been assigned a value
}`, "", "", 0), []map[string]interface{}{
			{
				"code":    "rego_unsafe_var_error",
				"message": "var z is unsafe",
			},
		}},
		{makeDR(`package example
p {
		x != 100
		x := 1     # error because x appears earlier in the query.
}

q {
		x := 1
		x := 2     # error because x is assigned twice.
}`, "", "", 0), []map[string]interface{}{
			{
				"code":    "rego_compile_error",
				"message": "var x referenced above",
			},
			{
				"code":    "rego_compile_error",
				"message": "var x assigned above",
			},
		}},
		{makeDR(`package example
r(1, x) = y {
		y := x
}

r(x, 2) = y {
		y := x*4
}`, "r(1, 2)", "", 0), map[string]interface{}{ // Non slice
			"code":    "eval_conflict_error",
			"message": "functions must not produce multiple outputs for same inputs",
		}},
		{makeDR("", `{"foo": y | z := [1, 2, 3]; y := z[_] }`, "", 0), map[string]interface{}{
			"code":    "eval_conflict_error",
			"message": "object keys must be unique",
		}},
	}

	for _, test := range tests {
		body, _ := json.Marshal(test.req)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body)) // These values don't matter
		NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "").handleQuery(w, r)
		var res apiError
		json.Unmarshal(w.Body.Bytes(), &res)
		if expErrs, ok := test.err.([]map[string]interface{}); ok {
			if resErrs, ok := res.Error.([]interface{}); ok {
				if len(expErrs) != len(resErrs) {
					t.Errorf("Eval resulted in wrong length slice error:\n\n%v\n\nRequest:\n\n%v", w.Body.String(), test.req)
				}

				for i := range expErrs {
					verifyError(t, expErrs[i], resErrs[i])
				}
			} else {
				t.Errorf("Eval resulted in non-slice error:\n\n%v\n\nRequest:\n\n%v", w.Body.String(), test.req)
			}
		} else {
			verifyError(t, test.err, res.Error)
		}
	}
}

func verifyError(t *testing.T, expected interface{}, actual interface{}) {
	t.Helper()

	eErr := expected.(map[string]interface{})
	aErr, ok := actual.(map[string]interface{})
	if !ok {
		t.Errorf("Unable to cast eval error: %v", actual)
	}

	if eErr["code"] != aErr["code"] || eErr["message"] != aErr["message"] {
		t.Errorf("Error code/message do not correspond:\n\n%v\n\ninstead of\n\n%v", actual, expected)
	}
}

// Makes a DataRequest from optionally empty (indicating not set) strings containing a single module, a query, and JSON input. If no module is provided, uses one with just a package declaration.
func makeDR(module string, query string, input string, regoVersion int, data ...string) DataRequest {
	out := DataRequest{}

	if module != "" {
		out.RegoModules = map[string]interface{}{
			"play/play.rego": module,
		}
	} else {
		out.RegoModules = map[string]interface{}{
			"play/play.rego": "package play",
		}
	}

	if query != "" {
		out.RegoQuery = query
	}

	if input != "" {
		util.UnmarshalJSON([]byte(input), &out.Input)
	}

	if len(data) > 0 {
		util.UnmarshalJSON([]byte(data[0]), &out.Data)
	}

	out.RegoVersion = &regoVersion

	return out
}

func TestApiNotFoundStatuses(t *testing.T) {
	api := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

	for _, path := range []string{
		"/bundles/does-not-exist",
		"/v1/input/does-not-exist",
		"/v1/data/does-not-exist",
	} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", path, nil)
			api.router.ServeHTTP(w, r)

			// resp status
			if exp, act := http.StatusNotFound, w.Code; exp != act {
				t.Errorf("expected response code %d, got %d", exp, act)
			}

			// resp body
			var res apiError
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			if exp, act := apiCodeNotFound, res.Code; exp != act {
				t.Errorf("expected response code %q, got %q", exp, act)
			}
		})
	}
}

func TestDoHandleRetrieveBundleRegularPoll(t *testing.T) {
	dr := makeDR(`
		package test

		p { print("hello", "world") }`, `p`, `{}`, 0)

	data := map[string]interface{}{"foo": "bar"}
	dr.Data = util.Reference(data)

	store := NewMemoryDataRequestStore()
	s := NewAPIService("", store, nil, "./", "", "", "")
	key := StoreKey{Id: "foo"}

	// no bundle
	w := httptest.NewRecorder()
	s.doHandleRetrieveBundle(w, &key, "", 0, []string{}, nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected status code %v, but got %v", http.StatusNotFound, w.Code)
	}

	dr.Etag = key.Id
	_, err := store.Put(&key, dr, nil)
	if err != nil {
		t.Fatal(err)
	}

	// unmodified bundle
	w = httptest.NewRecorder()
	s.doHandleRetrieveBundle(w, &key, key.Id, 0, []string{}, nil)

	if w.Code != http.StatusNotModified {
		t.Fatalf("Expected status code %v, but got %v", http.StatusNotModified, w.Code)
	}

	// new snapshot bundle available
	w = httptest.NewRecorder()
	s.doHandleRetrieveBundle(w, &key, "", 0, []string{}, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status code %v, but got %v", http.StatusOK, w.Code)
	}

	loader := bundle.NewTarballLoaderWithBaseURL(w.Body, "")
	bundl, err := bundle.NewCustomReader(loader).Read()
	if err != nil {
		t.Fatal(err)
	}

	if bundl.Manifest.Revision != dr.Etag {
		t.Fatalf("Expected bundle revision %v, but got %v", dr.Etag, bundl.Manifest.Revision)
	}

	// new delta bundle available
	patch := jsonpatch.JsonPatchOperation{
		Operation: "add",
		Path:      "/a/c/d",
		Value:     []string{"foo", "bar"},
	}

	dr.Patch = &[]jsonpatch.JsonPatchOperation{patch}
	_, err = store.Put(&key, dr, nil)
	if err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	s.doHandleRetrieveBundle(w, &key, "bar", 0, []string{deltaBundleMode}, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status code %v, but got %v", http.StatusOK, w.Code)
	}

	loader = bundle.NewTarballLoaderWithBaseURL(w.Body, "")
	b, err := bundle.NewCustomReader(loader).Read()
	if err != nil {
		t.Fatal(err)
	}

	if len(b.Patch.Data) != 1 {
		t.Fatalf("expected one patch but got %v", len(b.Patch.Data))
	}

	if b.Manifest.Revision != dr.Etag {
		t.Fatalf("Expected bundle revision %v, but got %v", dr.Etag, b.Manifest.Revision)
	}
}

func TestDoHandleRetrieveBundleLongPoll(t *testing.T) {
	dr := makeDR(`
		package test

		p { print("hello", "world") }`, `p`, `{}`, 0)

	data := map[string]interface{}{"foo": "bar"}
	dr.Data = util.Reference(data)

	store := NewMemoryDataRequestStore()
	s := NewAPIService("", store, nil, "./", "", "", "")
	key := StoreKey{Id: "foo"}

	dr.Etag = key.Id
	_, err := store.Put(&key, dr, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.doHandleRetrieveBundle(w, &key, "bar", 1*time.Second, []string{}, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status code %v, but got %v", http.StatusOK, w.Code)
	}

	loader := bundle.NewTarballLoaderWithBaseURL(w.Body, "")
	_, err = bundle.NewCustomReader(loader).Read()
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoHandleUpdateDistributeWithPatch(t *testing.T) {
	key := StoreKey{Id: "foo"}

	store := NewMemoryDataRequestStore()
	s := NewAPIService("", store, nil, "./", "", "", "")

	dr := makeDR(`
		package test

		p { print("hello", "world") }`, `p`, `{}`, 0)

	data := map[string]interface{}{"foo": "bar", "hello": "world"}
	dr.Data = util.Reference(data)
	body, _ := json.Marshal(dr)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/data", bytes.NewReader(body))

	dr2 := makeDR(`
		package test

		p { print("hello", "world") }`, `p`, `{}`, 0)

	data2 := map[string]interface{}{"foo": "qux"}
	dr2.Data = util.Reference(data2)
	dr2.Etag = key.Id

	_, err := store.Put(&key, dr2, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.doHandleUpdateDistribute(w, r, &key)

	// verify the data patch
	res, found, err := store.Get(&key, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatalf("key %v not found in store", key)
	}

	if res.Patch == nil {
		t.Fatal("expected data patch")
	}

	if len(*res.Patch) != 2 {
		t.Fatalf("expected two data patches but got %v", len(*res.Patch))
	}
}

func TestApiFormat(t *testing.T) {
	testCases := map[string]struct {
		RegoModule         string
		ExpectedRegoModule string
		RegoVersion        *int
	}{
		"nil rego version": {
			RegoModule: `package test

default allow = false

allow {
 input.foo  == "bar"
}`,
			ExpectedRegoModule: `package test

import rego.v1

default allow := false

allow if {
    input.foo == "bar"
}
`,
			RegoVersion: nil,
		},
		"rego version 0": {
			RegoModule: `package test
default allow = false

allow  {
    input.foo   == "bar"
}`,
			ExpectedRegoModule: `package test

import rego.v1

default allow := false

allow if {
    input.foo == "bar"
}
`,
			RegoVersion: &[]int{0}[0],
		},
		"rego version 1": {
			RegoModule: `package test
default allow = false
allow if { input.foo == "bar" }`,
			ExpectedRegoModule: `package test

default allow := false

allow if input.foo == "bar"
`,
			RegoVersion: &[]int{1}[0],
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			req := FormatRequest{
				RegoModule:  tc.RegoModule,
				RegoVersion: tc.RegoVersion,
			}

			body, _ := json.Marshal(req)
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/v1/fmt", bytes.NewReader(body))
			s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

			s.handleFormatting(w, r)

			if w.Code != 200 {
				t.Fatalf("expected 200 response but got: %v", w.Code)
			}

			var res FormatResponse
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Logf("body: %s", w.Body.String())
				t.Fatalf("failed to unmarshal response: %s", err)
			}

			actualLines := strings.Split(strings.TrimSpace(res.Result), "\n")
			expectedLines := strings.Split(strings.TrimSpace(tc.ExpectedRegoModule), "\n")

			for i := range actualLines {
				if i >= len(expectedLines) || strings.TrimSpace(actualLines[i]) != strings.TrimSpace(expectedLines[i]) {
					t.Errorf("line %d mismatch, got: %s, expected: %s", i+1, actualLines[i], expectedLines[i])
					return
				}
			}

			if len(actualLines) != len(expectedLines) {
				t.Errorf("number of lines mismatch, got: %d, expected: %d", len(actualLines), len(expectedLines))
			}
		})
	}
}

type MockAuth struct {
	token string
	check bool
}

func NewMockAuth(token string, check bool) *MockAuth {
	return &MockAuth{
		token: token,
		check: check,
	}
}

func (m *MockAuth) Token(_ context.Context, _ *oauth2.Token) (*oauth2.Token, error) {
	if m.token == "invalid" {
		return nil, fmt.Errorf("invalid token")
	}

	return &oauth2.Token{AccessToken: m.token}, nil
}

func (m *MockAuth) Exchange(_ context.Context, _ string) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: m.token}, nil
}

func (m *MockAuth) Check(_ context.Context, _ *oauth2.Token) bool {
	return m.check
}

func TestHandleTestAuth(t *testing.T) {
	tests := []struct {
		name          string
		authParam     string
		cookie        bool
		expectedCode  int
		expectedToken string
		check         bool
	}{
		{
			name:         "missing auth cookie",
			expectedCode: http.StatusUnauthorized,
			check:        true,
		},
		{
			name:          "successful flow",
			cookie:        true,
			expectedToken: "123",
			expectedCode:  http.StatusOK,
			check:         true,
		},
		{
			name:          "invalid token",
			cookie:        true,
			expectedToken: "invalid",
			expectedCode:  http.StatusUnauthorized,
			check:         true,
		},
		{
			name:          "revoked token",
			cookie:        true,
			expectedToken: "invalid",
			expectedCode:  http.StatusUnauthorized,
			check:         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse("/v2/auth/test")
			if err != nil {
				t.Fatal(err)
			}

			if tc.authParam != "" {
				q := u.Query()
				q.Set("auth", tc.authParam)
				u.RawQuery = q.Encode()
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", u.String(), nil)
			s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")
			s.auth = NewMockAuth(tc.expectedToken, tc.check)

			if tc.cookie {
				cookieToken := oauth2.Token{}

				b, err := json.Marshal(cookieToken)
				if err != nil {
					t.Fatal(err)
				}

				cookie := &http.Cookie{
					Name:  githubAuthCookie,
					Value: base64.StdEncoding.EncodeToString(b),
				}

				r.AddCookie(cookie)
			}

			s.testAuth(w, r)

			if w.Code != tc.expectedCode {
				t.Fatalf("expected %v but got %v", tc.expectedCode, w.Code)
			}

			if tc.expectedToken != "" && tc.expectedToken != "invalid" {
				rawCookies := w.Header().Get("Set-Cookie")
				header := http.Header{}
				header.Add("Cookie", rawCookies)
				request := http.Request{Header: header}
				cookie, err := request.Cookie(githubAuthCookie)
				if err != nil {
					t.Fatal(err)
				}

				b, err := base64.StdEncoding.DecodeString(cookie.Value)
				if err != nil {
					t.Fatal(err)
				}

				var cookieToken oauth2.Token
				if err := json.Unmarshal(b, &cookieToken); err != nil {
					t.Fatal(err)
				}

				if tc.expectedToken != cookieToken.AccessToken {
					t.Fatalf("expected %v but got %v", tc.expectedToken, cookie.Value)
				}
			}
		})
	}
}

func TestHandleGithubAuth(t *testing.T) {
	tests := []struct {
		name             string
		expectedCode     int
		expectedLocation string
	}{
		{
			name:             "always redirect",
			expectedCode:     http.StatusFound,
			expectedLocation: "https://github.com/login/oauth/authorize?client_id=&redirect_uri=%2Fv1%2Fgithubcallback&response_type=code&scope=gist",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/v2/auth", nil)
			s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")

			s.handleGithubAuth(w, r)

			if w.Code != tc.expectedCode {
				t.Fatalf("expected %v but got %v", tc.expectedCode, w.Code)
			}

			if w.Header().Get("Location") != tc.expectedLocation {
				t.Fatalf("expected %v but got %v", tc.expectedLocation, w.Header().Get("Location"))
			}
		})
	}
}

func TestHandleGithubCallback(t *testing.T) {
	tests := []struct {
		name             string
		authParam        string
		cookie           bool
		expectedCode     int
		expectedLocation string
		expectedToken    string
	}{
		{
			name:             "successful flow",
			expectedToken:    "123",
			expectedCode:     http.StatusFound,
			expectedLocation: "/v1/#auth=success",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/v1/githubcallback", nil)
			s := NewAPIService("", NewMemoryDataRequestStore(), nil, "./", "", "", "")
			s.auth = NewMockAuth(tc.expectedToken, true)

			s.handleGithubCallback(w, r)

			if w.Code != tc.expectedCode {
				t.Fatalf("expected %v but got %v", tc.expectedCode, w.Code)
			}

			if w.Header().Get("Location") != tc.expectedLocation {
				t.Fatalf("expected %v but got %v", tc.expectedLocation, w.Header().Get("Location"))
			}

			rawCookies := w.Header().Get("Set-Cookie")
			header := http.Header{}
			header.Add("Cookie", rawCookies)
			request := http.Request{Header: header}
			cookie, err := request.Cookie(githubAuthCookie)
			if err != nil {
				t.Fatal(err)
			}

			b, err := base64.StdEncoding.DecodeString(cookie.Value)
			if err != nil {
				t.Fatal(err)
			}

			var cookieToken oauth2.Token
			if err := json.Unmarshal(b, &cookieToken); err != nil {
				t.Fatal(err)
			}

			if tc.expectedToken != cookieToken.AccessToken {
				t.Fatalf("expected %v but got %v", tc.expectedToken, cookie.Value)
			}
		})
	}
}

func TestGistOpaque(t *testing.T) {
	tests := []struct {
		name            string
		ID              string
		Revision        string
		expectedEncoded string
	}{
		{
			name:            "successful encode and decode",
			ID:              "4f3d4eb9e5edb45fa5749dfea2ec2224",
			Revision:        "d99b29c358477f8945e031d15515d28d1ca22030",
			expectedEncoded: "g_NGYzZDRlYjllNWVkYjQ1ZmE1NzQ5ZGZlYTJlYzIyMjRf2Zspw1hHf4lF4DHRVRXSjRyiIDA",
		},
		{
			name:            "encoded string with multiple underscores",
			ID:              "88e75e0b485b9a5bade710320496bbb5",
			Revision:        "5068c7f988de64a47f7ace06cddefa5fd017faf7",
			expectedEncoded: "g_ODhlNzVlMGI0ODViOWE1YmFkZTcxMDMyMDQ5NmJiYjVfUGjH-YjeZKR_es4Gzd76X9AX-vc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := &StoreKey{
				Id:       tc.ID,
				Revision: tc.Revision,
				KeyType:  KeyTypeGist,
			}

			encoded, err := k.toOpaque()
			if err != nil {
				t.Fatal(err)
			}

			if encoded != tc.expectedEncoded {
				t.Fatalf("expected %v but got %v", tc.expectedEncoded, encoded)
			}

			decodedK, err := storeKeyFromOpaque(encoded)
			if err != nil {
				t.Fatal(err)
			}

			if tc.ID != decodedK.Id {
				t.Fatalf("expected %v but got %v", tc.ID, decodedK.Id)
			}
			if tc.Revision != decodedK.Revision {
				t.Fatalf("expected %v but got %v", tc.Revision, decodedK.Revision)
			}
		})
	}
}
