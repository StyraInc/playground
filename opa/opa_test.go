package opa

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/util"
	log "github.com/sirupsen/logrus"
)

// disable logging
func TestMain(m *testing.M) {
	log.SetLevel(log.ErrorLevel)
	os.Exit(m.Run())
}

// TestCompileWithFullUserInput tests compile result where
// all the input is provided by user via the Input box.
func TestCompileWithFullUserInput(t *testing.T) {

	ctx := context.Background()

	b := []byte(`{"message":"world"}`)
	var in interface{}
	json.Unmarshal(b, &in)

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	}`

	actual, _, err := Compile(ctx, &in, nil, map[string]string{"test.rego": policy}, "", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	expectedQuery, err := ast.ParseBody("data.play")
	if err != nil {
		t.Fatal(err)
	}

	if !actual.QueryParseResult.ParsedQuery.Equal(expectedQuery) {
		t.Fatalf("Expected query %v but got %v", expectedQuery, actual.QueryParseResult.ParsedQuery)
	}

	expectedInput, err := ast.InterfaceToValue(in)
	if err != nil {
		t.Fatal(err)
	}

	if expectedInput.Compare(actual.ParsedInput) != 0 {
		t.Fatalf("Expected input %v but got %v", expectedInput, actual.ParsedInput)
	}
}

// TestCompileWithMockFullInput tests compile result where
// all the input is provided by user by mocking whole input document.
func TestCompileWithMockFullInput(t *testing.T) {

	ctx := context.Background()

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}

	test_allow {
		hello with input as {"message": "world", "foo": "bar"}
	}`

	actual, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, "test_allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	expectedQuery, err := ast.ParseBody("test_allow")
	if err != nil {
		t.Fatal(err)
	}

	if !actual.QueryParseResult.ParsedQuery.Equal(expectedQuery) {
		t.Fatalf("Expected query %v but got %v", expectedQuery, actual.QueryParseResult.ParsedQuery)
	}

	if actual.ParsedInput != nil {
		t.Fatalf("Expected nil input but got %v", actual.ParsedInput)
	}
}

// TestCompileWithMockFullInputDotNotation tests compile result where
// all the input is provided by user by mocking specific fields
// of input document.
func TestCompileWithMockFullInputDotNotation(t *testing.T) {

	ctx := context.Background()

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}

	test_allow {
		hello with input.message as "world" with input.foo as "bar"
	}`

	actual, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, "test_allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	expectedQuery, err := ast.ParseBody("test_allow")
	if err != nil {
		t.Fatal(err)
	}

	if !actual.QueryParseResult.ParsedQuery.Equal(expectedQuery) {
		t.Fatalf("Expected query %v but got %v", expectedQuery, actual.QueryParseResult.ParsedQuery)
	}

	if actual.ParsedInput != nil {
		t.Fatalf("Expected nil input but got %v", actual.ParsedInput)
	}
}

// TestCompileWithMockPartialInputDotNotation tests compile result where
// the input is provided by user by mocking specific fields
// of input document and via the Input box.
func TestCompileWithMockPartialInputDotNotation(t *testing.T) {

	ctx := context.Background()

	b := []byte(`{"message":"world"}`)
	var in interface{}
	json.Unmarshal(b, &in)

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}

	test_allow {
		hello with input.foo as "bar"
	}`

	actual, _, err := Compile(ctx, &in, nil, map[string]string{"test.rego": policy}, "test_allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	expectedQuery, err := ast.ParseBody("test_allow")
	if err != nil {
		t.Fatal(err)
	}

	if !actual.QueryParseResult.ParsedQuery.Equal(expectedQuery) {
		t.Fatalf("Expected query %v but got %v", expectedQuery, actual.QueryParseResult.ParsedQuery)
	}

	expectedInput, err := ast.InterfaceToValue(in)
	if err != nil {
		t.Fatal(err)
	}

	if expectedInput.Compare(actual.ParsedInput) != 0 {
		t.Fatalf("Expected input %v but got %v", expectedInput, actual.ParsedInput)
	}
}

func TestCompileWithPackageUserInput(t *testing.T) {

	ctx := context.Background()

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}`

	actual, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, "package play", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	expectedQuery, err := ast.ParseBody("data.play")
	if err != nil {
		t.Fatal(err)
	}

	if !actual.QueryParseResult.ParsedQuery.Equal(expectedQuery) {
		t.Fatalf("Expected query %v but got %v", expectedQuery, actual.QueryParseResult.ParsedQuery)
	}
}

func TestCompileWithRuleBodyUserInput(t *testing.T) {

	ctx := context.Background()

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}`

	query := `default hello = false
	hello {
	  input.message == "world"
	  input.foo == "bar"
	}`

	actual, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, query, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	expectedQuery, err := ast.ParseBody("__hello__ := {__x__ | __x__ := data.play.hello}")
	if err != nil {
		t.Fatal(err)
	}

	if !actual.QueryParseResult.ParsedQuery.Equal(expectedQuery) {
		t.Fatalf("Expected query %v but got %v", expectedQuery, actual.QueryParseResult.ParsedQuery)
	}
}

func TestCompileIgnoredUserInput(t *testing.T) {

	ctx := context.Background()

	policy := `package play
	# Some comment
	import input.message
	import input.message as foo
	a(x) = x {
		true
	}
	b(x, y) {
		true
	}
	c := 1`

	testCases := []struct {
		note            string
		query           string
		expectedError   string
		expectedQuery   string
		expectedIgnored []string
	}{
		{
			note: "lonely function with arg",
			query: `a(x) = x {
						true
					}`,
			expectedError:   "1 error occurred: rego_compile_error: empty query cannot be compiled",
			expectedIgnored: []string{"selection:1:1: Ignoring function definition for 'a' during evaluation."},
		},
		{
			note: "two lonely functions with arg",
			query: `a(x) = x {
						true
					}
					b(x, y) {
						true
					}`,
			expectedError: "1 error occurred: rego_compile_error: empty query cannot be compiled",
			expectedIgnored: []string{
				"selection:1:1: Ignoring function definition for 'a' during evaluation.",
				"selection:4:6: Ignoring function definition for 'b' during evaluation.",
			},
		},
		{
			note: "function with arg",
			query: `a(x) = x {
						true
					}
					b := 1`,
			expectedQuery:   "assign(b, 1)",
			expectedIgnored: []string{"selection:1:1: Ignoring function definition for 'a' during evaluation."},
		},
		{
			note:            "lonely import",
			query:           "import input.message",
			expectedError:   "1 error occurred: rego_compile_error: empty query cannot be compiled",
			expectedIgnored: []string{"selection:1:1: Ignoring `import` statement for 'message' during evaluation."},
		},
		{
			note: "import",
			query: `import input.message
					b := 1`,
			expectedQuery:   "assign(b, 1)",
			expectedIgnored: []string{"selection:1:1: Ignoring `import` statement for 'message' during evaluation."},
		},
		{
			note: "two imports",
			query: `import input.message
					import input.message as foo
					b := 1`,
			expectedQuery: "assign(b, 1)",
			expectedIgnored: []string{
				"selection:1:1: Ignoring `import` statement for 'message' during evaluation.",
				"selection:2:6: Ignoring `import` statement for 'foo' during evaluation.",
			},
		},
		{
			note:          "lonely comment",
			query:         "# Some comment",
			expectedError: "1 error occurred: rego_compile_error: empty query cannot be compiled",
		},
		{
			note: "comment",
			query: `# Some comment
					b := 1`,
			expectedQuery: "assign(b, 1)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.note, func(t *testing.T) {
			actual, ignored, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, tc.query, nil, nil, false, nil)

			if len(ignored) != len(tc.expectedIgnored) {
				t.Fatalf("Got warnings: %v, expected: %v", ignored, tc.expectedIgnored)
			} else if len(tc.expectedIgnored) > 0 {
				for _, expectedWarning := range tc.expectedIgnored {
					found := false
					for _, warning := range ignored {
						if expectedWarning == warning {
							found = true
						}
					}
					if !found {
						t.Fatalf("Got ignored: %v, expected: %v", ignored, tc.expectedIgnored)
					}
				}
			}

			if len(tc.expectedError) > 0 {
				if err == nil {
					t.Fatalf("expected error but got: %v", actual.QueryParseResult.ParsedQuery.String())
				}

				if err.Error() != tc.expectedError {
					t.Fatalf("expected '%s' error but got: %s", tc.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}

				expectedQuery, err := ast.ParseBody(tc.expectedQuery)
				if err != nil {
					t.Fatal(err)
				}

				if !actual.QueryParseResult.ParsedQuery.Equal(expectedQuery) {
					t.Fatalf("Expected query %v but got %v", expectedQuery, actual.QueryParseResult.ParsedQuery)
				}
			}
		})
	}
}

func TestEvalWithBuiltInErrors(t *testing.T) {
	ctx := context.Background()

	policy := `package play

import future.keywords.if

allow if 1 / 0 == 1

allow if 2 / 0 == 2
`
	expectedError := `2 errors occurred:
play.rego:5: eval_builtin_error: div: divide by zero
play.rego:7: eval_builtin_error: div: divide by zero`

	compileRes, _, err := Compile(ctx, nil, nil, map[string]string{"play.rego": policy}, "allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, evalErr := Eval(ctx, compileRes, EvalOptions{
		BuiltInErrorsAll: true,
	})
	if evalErr == nil {
		t.Fatal("expected error")
	}
	if exp, act := expectedError, evalErr.RawError.Error(); exp != act {
		t.Fatalf("unexpected error, expected: %s, got: %s", exp, act)
	}
}

func TestEvalWithStrictBuiltInErrors(t *testing.T) {
	ctx := context.Background()

	policy := `package play

import future.keywords.if

allow if 1 / 0 == 1

allow if 2 / 0 == 2
`
	expectedError := "play.rego:5: eval_builtin_error: div: divide by zero"

	compileRes, _, err := Compile(ctx, nil, nil, map[string]string{"play.rego": policy}, "allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, evalErr := Eval(ctx, compileRes, EvalOptions{
		BuiltInErrorsStrict: true,
		// even when all is set, strict takes precedence
		BuiltInErrorsAll: true,
	})
	if evalErr == nil {
		t.Fatal("expected error")
	}
	if exp, act := expectedError, evalErr.RawError.Error(); exp != act {
		t.Fatalf("unexpected error, expected: %s, got: %s", exp, act)
	}
}

func TestEvalWithStrictMode(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		note          string
		policy        string
		expectedError string
	}{
		{
			note: "unused imports",
			policy: `package play

import input.foo
import data.foo

x := 1
`,
			expectedError: "2 errors occurred:\ntest.rego:3: rego_compile_error: import input.foo unused\ntest.rego:4: rego_compile_error: import data.foo unused",
		},
		{
			note: "duplicate imports",
			policy: `package play

import input.foo
import data.foo

x := 1 { foo }
`,
			expectedError: "1 error occurred: test.rego:4: rego_compile_error: import must not shadow import input.foo",
		},
		{
			note: "unused local var",
			policy: `package play

p {
	x := 1
}
`,
			expectedError: "1 error occurred: test.rego:4: rego_compile_error: assigned var x unused",
		},
		{
			note: "any() usage",
			policy: `package play

x := any([true, false])
`,
			expectedError: "1 error occurred: test.rego:3: rego_type_error: deprecated built-in function calls in expression: any",
		},
		{
			note: "all() usage",
			policy: `package play

x := all([true, false])
`,
			expectedError: "1 error occurred: test.rego:3: rego_type_error: deprecated built-in function calls in expression: all",
		},
		{
			note: "input override",
			policy: `package play

input := 1
`,
			expectedError: "1 error occurred: test.rego:3: rego_compile_error: rules must not shadow input (use a different rule name)",
		},
		{
			note: "data override",
			policy: `package play

data := 1
`,
			expectedError: "1 error occurred: test.rego:3: rego_compile_error: rules must not shadow data (use a different rule name)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.note, func(t *testing.T) {
			_, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": tc.policy}, "data.play", nil, nil, true, nil)

			if err == nil {
				t.Fatal("expected error")
			}

			if err.Error() != tc.expectedError {
				t.Fatalf("expected '%s' error but got: %s", tc.expectedError, err.Error())
			}
		})
	}
}

// TestEvalWithFullUserInput tests eval result where
// all the input is provided by user via the Input box.
func TestEvalWithFullUserInput(t *testing.T) {

	ctx := context.Background()

	b := []byte(`{"message":"world"}`)
	var in interface{}
	json.Unmarshal(b, &in)

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	}`

	compileRes, _, err := Compile(ctx, &in, nil, map[string]string{"test.rego": policy}, "hello", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertEval(ctx, t, compileRes, "[[true]]")
}

// TestEvalWithMockFullInput tests eval result where
// all the input is provided by user by mocking whole input document.
func TestEvalWithMockFullInput(t *testing.T) {

	ctx := context.Background()

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}

	test_allow {
		hello with input as {"message": "world", "foo": "bar"}
	}`

	compileRes, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, "test_allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertEval(ctx, t, compileRes, "[[true]]")
}

// TestEvalWithMockFullInputDotNotation tests eval result where
// all the input is provided by user by mocking specific fields
// of input document.
func TestEvalWithMockFullInputDotNotation(t *testing.T) {

	ctx := context.Background()

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}

	test_allow {
		hello with input.message as "world" with input.foo as "bar"
	}`

	compileRes, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, "test_allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertEval(ctx, t, compileRes, "[[true]]")
}

// TestEvalWithMockPartialInputDotNotation tests eval result where
// the input is provided by user by mocking specific fields
// of input document and via the Input box.
func TestEvalWithMockPartialInputDotNotation(t *testing.T) {

	ctx := context.Background()

	b := []byte(`{"message":"world"}`)
	var in interface{}
	json.Unmarshal(b, &in)

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	  input.foo == "bar"
	}

	test_allow {
		hello with input.foo as "bar"
	}`

	compileRes, _, err := Compile(ctx, &in, nil, map[string]string{"test.rego": policy}, "test_allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertEval(ctx, t, compileRes, "[[true]]")
}

func TestEvalWithPackageUserInput(t *testing.T) {

	ctx := context.Background()

	b := []byte(`{"message":"world"}`)
	var in interface{}
	json.Unmarshal(b, &in)

	policy := `package play

	default hello = false

	hello {
	  input.message == "world"
	}`

	compileRes, _, err := Compile(ctx, &in, nil, map[string]string{"test.rego": policy}, "package play", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertEval(ctx, t, compileRes, `[[{"hello": true}]]`)
}

func TestEvalWithRuleBodyUserInput(t *testing.T) {

	ctx := context.Background()

	b := []byte(`{"message":"world"}`)
	var in interface{}
	json.Unmarshal(b, &in)

	policy := `package play

	import input.message

	default hello = false

	hello {
	  input.message == "world"
	}
	
	bye {false}`

	query := `hello {
	  input.message == "world"
	  input.foo == "bar"
	}
	
	bye {false}`

	compileRes, _, err := Compile(ctx, &in, nil, map[string]string{"test.rego": policy}, query, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertEvalWithBindings(ctx, t, compileRes, "[[true, true]]", `[{"hello": true}]`)
}

func TestEvalWithCoverage(t *testing.T) {

	ctx := context.Background()

	module := `package play

	default allow = false  # exited

	allow {     # not exited
		x = 1   # eval
		x = 2   # fail
		z = 3   # not eval
	}`

	var in interface{} = map[string]string{}
	c, _, err := Compile(ctx, &in, nil, map[string]string{"test.rego": module}, "allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err2 := Eval(ctx, c, EvalOptions{Cover: true})
	if err2 != nil {
		t.Fatal(err2)
	}

	if result.Coverage == nil {
		t.Fatal("Expected coverage to be set")
	}
}

func TestEvalWithContextCancel(t *testing.T) {

	ctx := context.Background()

	policy := `package play

	allow {
		net.cidr_expand("1.0.0.0/1")
	}
	`

	compileRes, _, err := Compile(ctx, nil, nil, map[string]string{"test.rego": policy}, "allow", nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, evalErr := Eval(ctx, compileRes, EvalOptions{})
	if evalErr == nil {
		t.Fatal("Expected error but got nil")
	}
}

func assertEval(ctx context.Context, t *testing.T, compileRes *CompileResult, expected string) {
	assertEvalWithBindings(ctx, t, compileRes, expected, "[{}]")
}

func assertEvalWithBindings(ctx context.Context, t *testing.T, compileRes *CompileResult,
	expectedExpressions string, expectedBindings string) {
	t.Helper()

	actual, err := Eval(ctx, compileRes, EvalOptions{})
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.RawError.Error())
	}

	rs := actual.Result
	assertResultSet(t, rs, expectedExpressions, expectedBindings)
}

func assertResultSet(t *testing.T, rs rego.ResultSet, expectedExpressions string, expectedBindings string) {
	t.Helper()
	expressions := []interface{}{}
	bindings := []interface{}{}

	for i := range rs {
		values := []interface{}{}
		for j := range rs[i].Expressions {
			values = append(values, rs[i].Expressions[j].Value)
		}
		expressions = append(expressions, values)
		bindings = append(bindings, map[string]interface{}(rs[i].Bindings))
	}

	if !reflect.DeepEqual(expressions, util.MustUnmarshalJSON([]byte(expectedExpressions))) {
		t.Fatalf("Expected expressions:\n\n%v\n\nGot:\n\n%v", expectedExpressions, expressions)
	}

	exp := util.MustUnmarshalJSON([]byte(expectedBindings))
	if !reflect.DeepEqual(bindings, exp) {
		t.Fatalf("Expected bindings:\n\n%v\n\nGot:\n\n%v", expectedBindings, bindings)
	}
}
