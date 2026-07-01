// Copyright (c) the go-ruby-mustache/mustache authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mustache

import (
	"embed"
	"encoding/json"
	"math/big"
	"testing"
)

// The mustache spec (github.com/mustache/spec) ships language-independent test
// files as JSON alongside YAML; the JSON form resolves the YAML anchors and
// needs no YAML parser, so it is embedded here and asserted byte-for-byte. This
// is a deterministic, ruby-free conformance suite that alone holds coverage.
//
//go:embed spec/*.json
var specFS embed.FS

// specFile is the top-level shape of a spec JSON file.
type specFile struct {
	Overview string     `json:"overview"`
	Tests    []specTest `json:"tests"`
}

// specTest is one spec example. Data is decoded as a generic JSON value and then
// converted to the package value model. A lambda's Data carries a {"__tag__":
// "code", …} marker which the harness replaces with a Go lambda keyed by test
// name (Go cannot eval the Ruby proc source the file ships).
type specTest struct {
	Name     string            `json:"name"`
	Desc     string            `json:"desc"`
	Data     json.RawMessage   `json:"data"`
	Template string            `json:"template"`
	Expected string            `json:"expected"`
	Partials map[string]string `json:"partials"`
}

// TestSpec runs every embedded spec file and asserts byte-identical output.
func TestSpec(t *testing.T) {
	files := []string{
		"comments", "interpolation", "sections", "inverted",
		"partials", "delimiters", "~lambdas",
	}
	total, passed := 0, 0
	for _, name := range files {
		raw, err := specFS.ReadFile("spec/" + name + ".json")
		if err != nil {
			t.Fatalf("read spec %s: %v", name, err)
		}
		var sf specFile
		if err := json.Unmarshal(raw, &sf); err != nil {
			t.Fatalf("parse spec %s: %v", name, err)
		}
		filePass, fileTotal := 0, len(sf.Tests)
		for _, tc := range sf.Tests {
			total++
			ok := t.Run(name+"/"+tc.Name, func(t *testing.T) {
				ctx := decodeData(t, name, tc)
				got, err := Render(tc.Template, ctx, tc.Partials)
				if err != nil {
					t.Fatalf("Render error: %v\ntemplate: %q", err, tc.Template)
				}
				if got != tc.Expected {
					t.Errorf("%s\n got: %q\nwant: %q\ntmpl: %q", tc.Desc, got, tc.Expected, tc.Template)
				}
			})
			if ok {
				passed++
				filePass++
			}
		}
		t.Logf("spec %-14s %d/%d", name, filePass, fileTotal)
	}
	t.Logf("spec total %d/%d", passed, total)
	if passed != total {
		t.Errorf("spec conformance %d/%d (want all)", passed, total)
	}
}

// decodeData converts a spec test's JSON data to the package value model,
// substituting a Go lambda for a {"__tag__":"code"} marker keyed by test name.
func decodeData(t *testing.T, file string, tc specTest) Value {
	t.Helper()
	var raw any
	if len(tc.Data) > 0 {
		if err := json.Unmarshal(tc.Data, &raw); err != nil {
			t.Fatalf("data unmarshal: %v", err)
		}
	}
	return convert(t, raw, tc.Name)
}

// convert maps a decoded JSON value to the package value model. JSON objects
// become map[string]any (a Ruby Hash keyed by string), arrays become []any, and
// numbers become int64 when integral (Ruby Integer) else float64. A code marker
// is replaced with its Go lambda.
func convert(t *testing.T, v any, testName string) Value {
	switch n := v.(type) {
	case map[string]any:
		if tag, _ := n["__tag__"].(string); tag == "code" {
			return lambdaFor(t, testName)
		}
		m := make(map[string]any, len(n))
		for k, val := range n {
			m[k] = convert(t, val, testName)
		}
		return m
	case []any:
		s := make([]any, len(n))
		for i, val := range n {
			s[i] = convert(t, val, testName)
		}
		return s
	case float64:
		if n == float64(int64(n)) {
			return int64(n)
		}
		return n
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
		f, _ := n.Float64()
		return f
	}
	return v
}

// lambdaFor returns the Go lambda implementing the spec's Ruby proc for the
// named ~lambdas test. Each mirrors the `ruby:` source in the spec file.
func lambdaFor(t *testing.T, name string) Value {
	switch name {
	case "Interpolation":
		return Lambda(func(string) Value { return "world" })
	case "Interpolation - Expansion":
		return Lambda(func(string) Value { return "{{planet}}" })
	case "Interpolation - Alternate Delimiters":
		return Lambda(func(string) Value { return "|planet| => {{planet}}" })
	case "Interpolation - Multiple Calls":
		calls := 0
		return Lambda(func(string) Value { calls++; return calls })
	case "Escaping":
		return Lambda(func(string) Value { return ">" })
	case "Section":
		return Lambda(func(text string) Value {
			if text == "{{x}}" {
				return "yes"
			}
			return "no"
		})
	case "Section - Expansion":
		return Lambda(func(text string) Value { return text + "{{planet}}" + text })
	case "Section - Alternate Delimiters":
		return Lambda(func(text string) Value { return text + "{{planet}} => |planet|" + text })
	case "Section - Multiple Calls":
		return Lambda(func(text string) Value { return "__" + text + "__" })
	case "Inverted Section":
		return Lambda(func(string) Value { return false })
	}
	t.Fatalf("no Go lambda for spec test %q", name)
	return nil
}

// TestSpecInterpolationMultipleCalls guards the stateful-counter lambda, whose
// int return must coerce to "1"/"2"/"3" across three calls.
func TestSpecInterpolationMultipleCalls(t *testing.T) {
	calls := 0
	ctx := map[string]any{"lambda": Lambda(func(string) Value { calls++; return calls })}
	got, err := Render("{{lambda}} == {{{lambda}}} == {{lambda}}", ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1 == 2 == 3" {
		t.Errorf("got %q", got)
	}
}

// verifyBig keeps the big.Int import wired to a real assertion so ToString's
// bignum branch is exercised by the spec-adjacent tests too.
func TestBigIntToString(t *testing.T) {
	bi, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	if ToString(bi) != "123456789012345678901234567890" {
		t.Errorf("bignum to_s = %q", ToString(bi))
	}
}
