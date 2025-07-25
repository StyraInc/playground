package presentation

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/open-policy-agent/opa/rego"
)

// PrettyResultString pretty-formats a result set as a string.
func PrettyResultString(rs rego.ResultSet) string {
	pretty := &strings.Builder{}
	prettyResult(pretty, rs, 0) // Swallow error, shouldn't happen while writing to a string builder.
	return pretty.String()
}

// --- Copied verbatim from opa/internal/presentation/presentation.go (update from there needed) ---
func prettyResult(w io.Writer, rs rego.ResultSet, limit int) error {
	if len(rs) == 1 && len(rs[0].Bindings) == 0 {
		if len(rs[0].Expressions) == 1 || allBoolean(rs[0].Expressions) {
			return JSON(w, rs[0].Expressions[0].Value)
		}
	}

	keys := generateResultKeys(rs)
	tableBindings := generateTableBindings(w, keys, rs, limit)
	if tableBindings.NumLines() > 0 {
		tableBindings.Render()
	}

	return nil
}

type resultKey struct {
	varName   string
	exprIndex int
	exprText  string
}

func (rk resultKey) string() string {
	if rk.varName != "" {
		return rk.varName
	}
	return rk.exprText
}

func resultKeyLess(a, b resultKey) bool {
	if a.varName != "" {
		if b.varName == "" {
			return true
		}
		return a.varName < b.varName
	}
	return a.exprIndex < b.exprIndex
}

func allBoolean(ev []*rego.ExpressionValue) bool {
	for i := range ev {
		if _, ok := ev[i].Value.(bool); !ok {
			return false
		}
	}
	return true
}

func generateResultKeys(rs rego.ResultSet) []resultKey {
	keys := []resultKey{}
	if len(rs) != 0 {
		for k := range rs[0].Bindings {
			keys = append(keys, resultKey{
				varName: k,
			})
		}

		for i, expr := range rs[0].Expressions {
			if _, ok := expr.Value.(bool); !ok {
				keys = append(keys, resultKey{
					exprIndex: i,
					exprText:  expr.Text,
				})
			}
		}

		sort.Slice(keys, func(i, j int) bool {
			return resultKeyLess(keys[i], keys[j])
		})
	}
	return keys
}

// JSON shouldn't be exported in this scenario but it is in the source for this file.
func JSON(w io.Writer, x interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(x)
}

func generateTableBindings(writer io.Writer, keys []resultKey, rs rego.ResultSet, prettyLimit int) *tablewriter.Table {
	table := tablewriter.NewWriter(writer)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetAutoFormatHeaders(false)
	header := make([]string, len(keys))
	for i := range header {
		header[i] = keys[i].string()
	}
	table.SetHeader(header)
	alignment := make([]int, len(keys))
	for i := range header {
		alignment[i] = tablewriter.ALIGN_LEFT
	}
	table.SetColumnAlignment(alignment)

	for _, row := range rs {
		printPrettyRow(table, keys, row, prettyLimit)
	}
	return table
}

func printPrettyRow(table *tablewriter.Table, keys []resultKey, result rego.Result, prettyLimit int) {
	buf := []string{}
	for _, k := range keys {
		v, ok := k.selectVarValue(result)
		if ok {
			js, err := json.Marshal(v)
			if err != nil {
				buf = append(buf, err.Error())
			} else {
				s := checkStrLimit(string(js), prettyLimit)
				buf = append(buf, s)
			}
		}
	}
	table.Append(buf)
}

func (rk resultKey) selectVarValue(result rego.Result) (interface{}, bool) {
	if rk.varName != "" {
		return result.Bindings[rk.varName], true
	}
	val := result.Expressions[rk.exprIndex].Value
	if _, ok := val.(bool); ok {
		return nil, false
	}
	return val, true
}

func checkStrLimit(input string, limit int) string {
	if limit > 0 && len(input) > limit {
		input = input[:limit] + "..."
		return input
	}
	return input
}
