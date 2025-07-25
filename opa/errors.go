package opa

import (
	"github.com/open-policy-agent/opa/ast"
)

// checkHTTPSendCompiler will come up with a nicer error in case the user input had
// a call to http.send in it. Since we've been removing that built-in function
// via capabilities, it would otherwise look like "unknown function: http.send".
// Registered as Compiler "AfterStage" in `Compile()`
func checkHTTPSendCompiler(c *ast.Compiler) *ast.Error {
	var errs ast.Errors
	for _, m := range c.Modules {
		ast.WalkExprs(m, checkExpr(&errs))
	}
	c.Errors = append(c.Errors, errs...)
	return nil
}

// Registered as QueryCompiler "AfterStage" in `Compile()`
func checkHTTPSend(_ ast.QueryCompiler, q ast.Body) (ast.Body, error) {
	var errs ast.Errors
	ast.WalkExprs(q, checkExpr(&errs))
	if len(errs) > 0 {
		return nil, errs
	}
	return q, nil
}

func checkExpr(errs *ast.Errors) func(expr *ast.Expr) bool {
	httpSend := ast.MustParseRef("http.send")

	return func(expr *ast.Expr) bool {
		// straightforward http.send call
		if expr.IsCall() && expr.Operator().Equal(httpSend) {
			*errs = append(*errs, ast.NewError(ast.TypeErr, expr.Location, "unsafe built-in function calls in expression: http.send"))
			return true
		}

		// with x as http.send without the http.send builtin _known_ will end up looking like
		//   __local0__ = http.send
		//   ... with x as __local0__
		// so that's what we capture here
		if expr.IsEquality() {
			r0, ok := expr.Operand(1).Value.(ast.Ref)
			if ok && r0.Equal(httpSend) {
				*errs = append(*errs, ast.NewError(ast.TypeErr, expr.Location, "unsafe built-in function calls in expression: http.send"))
				return true
			}
		}
		return false
	}
}
