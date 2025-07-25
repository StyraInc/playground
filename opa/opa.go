package opa

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/open-policy-agent/opa/ast"
	coverpkg "github.com/open-policy-agent/opa/cover"
	"github.com/open-policy-agent/opa/metrics"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/open-policy-agent/opa/topdown/print"
)

type Ignored = []string

var parserOptions = []struct {
	imp    *ast.Import
	setter func(*ast.ParserOptions)
}{
	{
		imp: ast.MustParseImports("import future.keywords")[0],
		setter: func(po *ast.ParserOptions) {
			po.AllFutureKeywords = true
		},
	},
	{
		imp: ast.MustParseImports("import future.keywords.in")[0],
		setter: func(po *ast.ParserOptions) {
			po.FutureKeywords = append(po.FutureKeywords, "in")
		},
	},
	{
		imp: ast.MustParseImports("import rego.v1")[0],
		setter: func(po *ast.ParserOptions) {
			po.RegoVersion = ast.RegoV1
		},
	},
}

var caps = capabilities()

func capabilities() *ast.Capabilities {
	caps := ast.CapabilitiesForThisVersion()

	excluded := map[string]bool{
		ast.HTTPSend.Name:        true,
		ast.NetLookupIPAddr.Name: true,
	}

	filtered := make([]*ast.Builtin, 0, len(caps.Builtins))
	for _, bi := range caps.Builtins {
		if !excluded[bi.Name] {
			filtered = append(filtered, bi)
		}
	}

	caps.Builtins = filtered
	return caps
}

type QueryParseResult struct {
	ParsedQuery      ast.Body
	adHocAssignments map[string]string
}

// CompileResult represents the result of the compile stage.
type CompileResult struct {
	QueryParseResult QueryParseResult
	Modules          map[string]*ast.Module
	Imports          []*ast.Import
	Package          *ast.Package
	Compiler         *ast.Compiler
	ParsedInput      ast.Value
	Store            storage.Store
}

// EvalOptions defines options for evaluation
type EvalOptions struct {
	DebugTrace          bool
	Cover               bool
	BuiltInErrorsAll    bool
	BuiltInErrorsStrict bool
}

// EvalResult represents the result of the evaluation function.
type EvalResult struct {
	Result   rego.ResultSet
	Time     int64
	Trace    []*topdown.Event
	Coverage *coverpkg.Report
	Output   string
}

// ParseResult represents the result of parsing a rego source text string
type ParseResult struct {
	Vars []string
}

// Error is the error type returned by the Eval function.
type Error struct {
	RawError   error
	HTTPStatus int
}

func newError() *Error {
	return &Error{}
}

// Compile compiles OPA query. There must be at least one policy.
func Compile(ctx context.Context, input *interface{}, data *interface{}, policies map[string]string, query string,
	queryPackage *string, queryImports *[]string, strict bool, regoVersion *int,
) (*CompileResult, Ignored, error) {
	var inputValue ast.Value
	var err error

	if input != nil {
		inputValue, err = ast.InterfaceToValue(*input)
		if err != nil {
			return nil, nil, err
		}
	}

	// To avoid a hard error on the frontend, we leave the store null if the
	// data document was null; otherwise, it is populated normally.
	var store storage.Store
	if data != nil {
		dataMap, ok := (*data).(map[string]interface{})
		if ok {
			store = inmem.NewFromObject(dataMap)
		}
	}

	regoVer := ast.DefaultRegoVersion
	if regoVersion != nil {
		regoVer = ast.RegoVersionFromInt(*regoVersion)
	}

	ms := make(map[string]*ast.Module, len(policies))
	for name, policy := range policies {
		m, err := ast.ParseModuleWithOpts(name, policy, ast.ParserOptions{RegoVersion: regoVer})
		if err != nil {
			return nil, nil, err
		}
		if m == nil {
			return nil, nil, fmt.Errorf("Invalid parameter: empty rego module")
		}
		ms[name] = m
	}

	// Extract one of the parsed modules (not a compiled module, otherwise imports are lost);
	// potentially used to determine query string, package, and imports.
	var one *ast.Module
	for _, module := range ms {
		one = module
		break
	}

	// Compile the modules, caching the result in the compiler
	compiler := ast.NewCompiler().
		WithCapabilities(caps).
		WithEnablePrintStatements(true).
		WithStrict(strict).
		WithStageAfter("RewriteWithValues", ast.CompilerStageDefinition{
			Name:       "CheckHTTPSend",
			MetricName: "compiler_stage_check_http_send",
			Stage:      checkHTTPSendCompiler,
		})
	compiler.Compile(ms)
	if compiler.Failed() {
		return nil, nil, compiler.Errors
	}

	// Choose a query if none provided.
	if query == "" {
		if len(compiler.Modules) == 1 {
			query = one.Package.Path.String()
		} else {
			query = "data"
		}
	}

	// Determine what package and imports to use
	var qP *ast.Package
	var qIs []*ast.Import

	switch {
	case queryPackage == nil:
		if len(compiler.Modules) == 1 { // Cannot infer package when more than one module
			qP = one.Package
		}

	case *queryPackage == "": // Set but empty case indicates that there should be no package
	default: // Set
		qP, err = ast.ParsePackage(fmt.Sprintf("package %v", *queryPackage))
		if err != nil {
			return nil, nil, err
		}
	}
	// qP may still be nil

	switch {
	case queryImports == nil:
		if len(compiler.Modules) == 1 { // Cannot infer imports when more than one module
			qIs = one.Imports
		}
	case len(*queryImports) == 0: // Set but empty case indicates that there should be no imports
	default: // Set
		// Taken from OPA's rego.go's compileQuery() method.
		s := make([]string, len(*queryImports))
		for i := range *queryImports {
			s[i] = fmt.Sprintf("import %v", (*queryImports)[i])
		}
		qIs, err = ast.ParseImports(strings.Join(s, "\n"))
		if err != nil {
			return nil, nil, err
		}
	}
	// qIs may still be nil

	// Apply any parser options based on opt-in via import.
	var opts ast.ParserOptions

	for _, qimp := range qIs {
		for _, po := range parserOptions {
			if po.imp.Equal(qimp) {
				po.setter(&opts)
			}
		}
	}

	// Parse and compile the query.
	queryParseResult, ignored, err := parseQuery(query, opts, one)
	if err != nil {
		return nil, ignored, err
	}
	qc := compiler.QueryCompiler().
		WithContext(ast.NewQueryContext().
			WithPackage(qP).
			WithImports(qIs)).
		WithStageAfter("RewriteWithValues", ast.QueryCompilerStageDefinition{
			Name:       "CheckHTTPSend",
			MetricName: "query_compile_stage_check_http_send",
			Stage:      checkHTTPSend,
		}).
		WithEnablePrintStatements(true)
	_, err = qc.Compile(queryParseResult.ParsedQuery)
	if err != nil {
		return nil, ignored, err
	}

	return &CompileResult{
		QueryParseResult: queryParseResult,
		Modules:          ms,
		Imports:          qIs,
		Package:          qP,
		Compiler:         compiler,
		ParsedInput:      inputValue,
		Store:            store,
	}, ignored, nil
}

func parseQuery(query string, opts ast.ParserOptions, one *ast.Module) (QueryParseResult, Ignored, error) {
	stmts, _, err := ast.ParseStatementsWithOpts("", query, opts)
	if err != nil {
		return QueryParseResult{}, nil, err
	}

	parsedQuery := ast.Body{}
	adHocAssignmentNames := make(map[string]struct{})
	adHocAssignments := make(map[string]string)
	ignored := make([]string, 0, 1)

	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.Package:
			// Package was selected; don't evaluate other statements in query
			return QueryParseResult{ParsedQuery: ast.Body{ast.NewExpr(&ast.Term{Value: stmt.Path})}}, nil, nil
		case ast.Body:
			for _, expr := range stmt {
				parsedQuery.Append(expr)
			}
		case *ast.Rule:
			name := stmt.Head.Name.String()

			// Ignore functions with arguments
			if len(stmt.Head.Args) > 0 {
				ignored = append(ignored, fmt.Sprintf("selection:%d:%d: Ignoring function definition for '%s' during evaluation.",
					stmt.Location.Row, stmt.Location.Col, name))
			} else if _, found := adHocAssignmentNames[name]; !found {
				assignedName := fmt.Sprintf("__%s__", name)
				expr, err := ast.ParseExpr(fmt.Sprintf("%s := { __x__ | __x__ := %s.%s }",
					assignedName, one.Package.Path.String(), name))
				if err != nil {
					log.WithError(err).Error("query construction error")
					ignored = append(ignored, fmt.Sprintf("selection:%d:%d: Ignoring `rule` '%s'; failed to construct assignment for query",
						stmt.Location.Row, stmt.Location.Col, name))
				}
				parsedQuery.Append(expr)
				adHocAssignmentNames[name] = struct{}{}
				adHocAssignments[assignedName] = name
			}
		case *ast.Import:
			var name string
			if len(stmt.Alias) > 0 {
				name = stmt.Alias.String()
			} else if tail, ok := tailOfImportPath(stmt); ok {
				name = tail
			} else {
				name = ""
			}
			ignored = append(ignored, fmt.Sprintf("selection:%d:%d: Ignoring `import` statement for '%s' during evaluation.",
				stmt.Location.Row, stmt.Location.Col, name))
		default:
			// skip
		}
	}

	return QueryParseResult{ParsedQuery: parsedQuery, adHocAssignments: adHocAssignments}, ignored, nil
}

func filterResultSet(resultSet rego.ResultSet, queryParseResult QueryParseResult) rego.ResultSet {
	filteredSet := rego.ResultSet{}
	for _, result := range resultSet {
		bindings := make(map[string]interface{})
		for name, binding := range result.Bindings {
			if actual, found := queryParseResult.adHocAssignments[name]; found {
				// This is a constructed ad-hoc query assignment, only add if comprehension result has >=1 values
				if arr, ok := binding.([]interface{}); ok {
					if len(arr) >= 1 {
						bindings[actual] = arr[0]
					}
					if len(arr) > 1 {
						log.Warningf("Ad-hoc query assignment for '%s' comprehension result had more than one entries", actual)
					}
				}
			} else {
				bindings[name] = binding
			}
		}
		filteredSet = append(filteredSet, rego.Result{Bindings: bindings, Expressions: result.Expressions})
	}

	return filteredSet
}

func tailOfImportPath(imp *ast.Import) (string, bool) {
	if path, ok := imp.Path.Value.(ast.Ref); ok && len(path) > 0 {
		tail := path[len(path)-1]
		if val, ok := tail.Value.(ast.String); ok {
			return string(val), true
		}
	}

	return "", false
}

type printHook struct {
	w io.Writer
}

func (ph printHook) Print(pctx print.Context, msg string) error {
	fmt.Fprintf(ph.w, "%v: %v\n", pctx.Location, msg)
	return nil
}

type WriteableBuiltInErrors []topdown.Error

func (e WriteableBuiltInErrors) Error() string {
	if len(e) == 0 {
		return "no error(s)"
	}

	if len(e) == 1 {
		return fmt.Sprintf("1 error occurred: %v", e[0].Error())
	}

	s := make([]string, len(e))
	for i, err := range e {
		s[i] = err.Error()
	}

	return fmt.Sprintf("%d errors occurred:\n%s", len(e), strings.Join(s, "\n"))
}

// Eval evaluates OPA query.
func Eval(ctx context.Context, input *CompileResult, options EvalOptions) (*EvalResult, *Error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	evalError := newError()

	defer func() {
		if r := recover(); r != nil {
			var ok bool
			err, ok := r.(error)
			evalError.HTTPStatus = http.StatusInternalServerError
			if !ok {
				evalError.RawError = fmt.Errorf("interface conversion error")
			} else {
				evalError.RawError = err
			}
		}
	}()

	met := metrics.New()

	var builtInErrorList *[]topdown.Error
	if options.BuiltInErrorsAll {
		builtInErrorList = &[]topdown.Error{}
	}

	var buf bytes.Buffer

	regoArgs := []func(*rego.Rego){
		rego.ParsedQuery(input.QueryParseResult.ParsedQuery),
		rego.Store(input.Store),
		rego.ParsedImports(input.Imports),
		rego.ParsedPackage(input.Package),
		rego.Compiler(input.Compiler),
		rego.BuiltinErrorList(builtInErrorList),
		rego.StrictBuiltinErrors(options.BuiltInErrorsStrict),
		rego.Metrics(met),
		rego.EnablePrintStatements(true),
		rego.PrintHook(printHook{w: &buf}),
		rego.Runtime(ast.ObjectTerm([2]*ast.Term{
			ast.StringTerm("message"),
			ast.StringTerm("The Rego Playground does not provide OPA runtime information during policy execution."),
		})),
	}

	now := time.Now()
	seed := now.UnixNano()

	evalArgs := []rego.EvalOption{
		rego.EvalParsedInput(input.ParsedInput),
		rego.EvalMetrics(met),
		rego.EvalSortSets(true),
		rego.EvalTime(now),
	}

	var tracer *topdown.BufferTracer

	if options.DebugTrace {
		tracer = topdown.NewBufferTracer()
		evalArgs = append(evalArgs, rego.EvalTracer(tracer), rego.EvalRuleIndexing(false))
	}

	var cover *coverpkg.Cover

	if options.Cover {
		cover = coverpkg.New()
		evalArgs = append(evalArgs, rego.EvalTracer(cover))
	}

	r := rego.New(regoArgs...)

	var rs rego.ResultSet

	pq, err := r.PrepareForEval(ctx)
	if err != nil {
		evalError = handleTopdownErr(err)
		return nil, evalError
	}

	rs, err = pq.Eval(ctx, append(evalArgs, rego.EvalSeed(rand.New(rand.NewSource(seed))))...)
	if err != nil {
		evalError = handleTopdownErr(err)
		return nil, evalError
	}

	if builtInErrorList != nil && len(*builtInErrorList) > 0 {
		var writeableErrors WriteableBuiltInErrors
		for _, err := range *builtInErrorList {
			writeableErrors = append(writeableErrors, err)
		}

		evalError = handleTopdownErr(writeableErrors)
		return nil, evalError
	}

	result := EvalResult{}
	result.Result = filterResultSet(rs, input.QueryParseResult)

	var ok bool
	result.Time, ok = met.All()["timer_rego_query_eval_ns"].(int64)
	if !ok {
		evalError.HTTPStatus = http.StatusInternalServerError
		evalError.RawError = fmt.Errorf("interface conversion error")
		return nil, evalError
	}

	if tracer != nil {
		result.Trace = *tracer
	}

	if cover != nil {
		report := cover.Report(input.Modules)
		result.Coverage = &report
	}

	if buf.Len() > 0 {
		result.Output = buf.String()
	}

	return &result, nil
}

func VarsForSelection(rawModule string, rawSelection string) (*ParseResult, *Error) {
	// Make sure the selection parses first.. don't bother with anything else if it isn't valid.
	body, err := ast.ParseBody(rawSelection)
	if err != nil {
		return nil, handleTopdownErr(err)
	}

	compiler := ast.NewCompiler()

	mod, err := ast.ParseModule("policy.rego", rawModule)
	if err != nil {
		return nil, handleTopdownErr(err)
	}

	compiler.Compile(map[string]*ast.Module{"policy.rego": mod})
	if compiler.Failed() {
		return nil, handleTopdownErr(compiler.Errors)
	}

	var resolved ast.Body
	shortCircuit := ast.QueryCompilerStageDefinition{
		Name:       "ShortCircuit",
		MetricName: "compiler_stage_short_circuit",
		Stage: func(_ ast.QueryCompiler, b ast.Body) (ast.Body, error) {
			resolved = b.Copy()
			return b, errors.New("STOP")
		},
	}

	qctx := ast.NewQueryContext()
	qctx.Package = mod.Package

	qc := compiler.QueryCompiler().
		WithStageAfter("RewriteWithValues", shortCircuit).
		WithContext(qctx)

	_, err = qc.Compile(body)
	if err != nil && err.Error() != "STOP" {
		return nil, handleTopdownErr(err)
	}

	result := &ParseResult{}

	for _, v := range resolved.Vars(ast.VarVisitorParams{SkipRefHead: true, SkipRefCallHead: true}).Sorted() {
		if rewritten, ok := qc.RewrittenVars()[v]; ok {
			v = rewritten
		}
		if v.IsWildcard() {
			continue
		}
		result.Vars = append(result.Vars, v.String())
	}

	return result, nil
}

func handleTopdownErr(err error) *Error {
	evalError := newError()
	evalError.RawError = err

	_, ok := err.(WriteableBuiltInErrors)
	if ok {
		evalError.HTTPStatus = http.StatusBadRequest
		return evalError
	}

	code, ok := err.(*topdown.Error)
	if !ok {
		evalError.HTTPStatus = http.StatusBadRequest
	} else {
		if code.Code == topdown.CancelErr || code.Code == topdown.InternalErr {
			evalError.HTTPStatus = http.StatusInternalServerError
		} else {
			evalError.HTTPStatus = http.StatusBadRequest
		}
	}

	return evalError
}
