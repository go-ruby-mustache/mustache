// Copyright (c) the go-ruby-mustache/mustache authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mustache

import (
	"math/big"
	"strings"
	"testing"
)

// obj is a test Object (a Mustache view) exposing named methods.
type obj struct{ m map[string]Value }

func (o obj) Method(name string) (Value, bool) { v, ok := o.m[name]; return v, ok }

// mustRender renders and fails the test on error.
func mustRender(t *testing.T, tmpl string, ctx Value, partials map[string]string) string {
	t.Helper()
	out, err := Render(tmpl, ctx, partials)
	if err != nil {
		t.Fatalf("Render(%q): %v", tmpl, err)
	}
	return out
}

func TestRenderBasic(t *testing.T) {
	cases := []struct {
		name, tmpl, want string
		ctx              Value
	}{
		{"plain text", "hello", "hello", nil},
		{"var", "{{x}}", "v", map[string]any{"x": "v"}},
		{"missing var", "[{{x}}]", "[]", map[string]any{}},
		{"escape", "{{x}}", "&amp;&lt;&gt;&quot;&#39;", map[string]any{"x": `&<>"'`}},
		{"raw triple", "{{{x}}}", `&<>`, map[string]any{"x": `&<>`}},
		{"raw amp", "{{&x}}", `&<>`, map[string]any{"x": `&<>`}},
		{"symbol key", "{{x}}", "v", map[Symbol]any{Symbol("x"): "v"}},
		{"implicit iterator", "{{#l}}{{.}}{{/l}}", "ab", map[string]any{"l": []any{"a", "b"}}},
		{"dotted", "{{a.b}}", "v", map[string]any{"a": map[string]any{"b": "v"}}},
		{"dotted broken", "[{{a.b.c}}]", "[]", map[string]any{"a": map[string]any{}}},
		{"int", "{{x}}", "42", map[string]any{"x": 42}},
		{"int64", "{{x}}", "7", map[string]any{"x": int64(7)}},
		{"float", "{{x}}", "2.5", map[string]any{"x": 2.5}},
		{"float integral", "{{x}}", "2.0", map[string]any{"x": 2.0}},
		{"float32", "{{x}}", "1.5", map[string]any{"x": float32(1.5)}},
		{"bool true", "{{x}}", "true", map[string]any{"x": true}},
		{"bool false", "{{x}}", "false", map[string]any{"x": false}},
		{"nil var", "[{{x}}]", "[]", map[string]any{"x": nil}},
		{"symbol value", "{{x}}", "sym", map[string]any{"x": Symbol("sym")}},
		{"bignum", "{{x}}", "10", map[string]any{"x": big.NewInt(10)}},
		{"comment", "a{{! hi }}b", "ab", nil},
		{"section truthy", "{{#x}}Y{{/x}}", "Y", map[string]any{"x": true}},
		{"section false", "{{#x}}Y{{/x}}", "", map[string]any{"x": false}},
		{"section missing", "{{#x}}Y{{/x}}", "", map[string]any{}},
		{"section nil", "{{#x}}Y{{/x}}", "", map[string]any{"x": nil}},
		{"section hash", "{{#x}}{{y}}{{/x}}", "v", map[string]any{"x": map[string]any{"y": "v"}}},
		{"section string", "{{#x}}Y{{/x}}", "Y", map[string]any{"x": "s"}},
		{"section empty list", "{{#x}}Y{{/x}}", "", map[string]any{"x": []any{}}},
		{"inverted absent", "{{^x}}N{{/x}}", "N", map[string]any{}},
		{"inverted false", "{{^x}}N{{/x}}", "N", map[string]any{"x": false}},
		{"inverted empty list", "{{^x}}N{{/x}}", "N", map[string]any{"x": []any{}}},
		{"inverted truthy", "{{^x}}N{{/x}}", "", map[string]any{"x": true}},
		{"inverted list", "{{^x}}N{{/x}}", "", map[string]any{"x": []any{1}}},
		{"setdelim", "{{=<% %>=}}<%x%>", "v", map[string]any{"x": "v"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mustRender(t, c.tmpl, c.ctx, nil); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestObjectView(t *testing.T) {
	ctx := obj{m: map[string]Value{"name": "Bob", "on": true}}
	if got := mustRender(t, "{{name}} {{#on}}Y{{/on}}", ctx, nil); got != "Bob Y" {
		t.Errorf("got %q", got)
	}
	// A missing method yields the empty string.
	if got := mustRender(t, "[{{gone}}]", ctx, nil); got != "[]" {
		t.Errorf("got %q", got)
	}
}

func TestOrderedMapContext(t *testing.T) {
	m := NewMap()
	m.Set("a", "1")
	m.Set(Symbol("b"), "2")
	m.Set("a", "1x") // replace
	if got := mustRender(t, "{{a}}{{b}}", m, nil); got != "1x2" {
		t.Errorf("got %q", got)
	}
	if m.Len() != 2 {
		t.Errorf("len = %d", m.Len())
	}
	if len(m.Pairs()) != 2 {
		t.Errorf("pairs = %d", len(m.Pairs()))
	}
	if v, ok := m.Get("a"); !ok || v != "1x" {
		t.Errorf("get a = %v,%v", v, ok)
	}
	if _, ok := m.Get("zzz"); ok {
		t.Errorf("get missing should fail")
	}
	// Symbol-only key resolves when the template names it without the colon.
	m2 := NewMap()
	m2.Set(Symbol("only"), "s")
	if got := mustRender(t, "{{only}}", m2, nil); got != "s" {
		t.Errorf("symbol-only got %q", got)
	}
}

func TestPartials(t *testing.T) {
	got := mustRender(t, "{{>p}}", map[string]any{"x": "v"}, map[string]string{"p": "[{{x}}]"})
	if got != "[v]" {
		t.Errorf("got %q", got)
	}
	// Missing partial → empty.
	if got := mustRender(t, "[{{>gone}}]", nil, nil); got != "[]" {
		t.Errorf("missing partial got %q", got)
	}
	// Empty partial → empty.
	if got := mustRender(t, "[{{>e}}]", nil, map[string]string{"e": ""}); got != "[]" {
		t.Errorf("empty partial got %q", got)
	}
	// Standalone partial indentation.
	got = mustRender(t, "  {{>p}}\n", nil, map[string]string{"p": "a\nb\n"})
	if got != "  a\n  b\n" {
		t.Errorf("indent got %q", got)
	}
	// Recursion.
	data := map[string]any{"content": "X", "nodes": []any{map[string]any{"content": "Y", "nodes": []any{}}}}
	got = mustRender(t, "{{>node}}", data, map[string]string{"node": "{{content}}<{{#nodes}}{{>node}}{{/nodes}}>"})
	if got != "X<Y<>>" {
		t.Errorf("recursion got %q", got)
	}
}

func TestLambdaShapes(t *testing.T) {
	// Lambda type.
	if got := mustRender(t, "{{x}}", map[string]any{"x": Lambda(func(string) Value { return "L" })}, nil); got != "L" {
		t.Errorf("Lambda got %q", got)
	}
	// func(string) any.
	fs := func(s string) any { return s + s }
	if got := mustRender(t, "{{#x}}b{{/x}}", map[string]any{"x": fs}, nil); got != "bb" {
		t.Errorf("func(string) got %q", got)
	}
	// func() any.
	f0 := func() any { return "F" }
	if got := mustRender(t, "{{x}}", map[string]any{"x": f0}, nil); got != "F" {
		t.Errorf("func() got %q", got)
	}
	// Interpolation lambda result is rendered against default delimiters.
	ctx := map[string]any{"x": Lambda(func(string) Value { return "{{y}}" }), "y": "Z"}
	if got := mustRender(t, "{{x}}", ctx, nil); got != "Z" {
		t.Errorf("interp-lambda expansion got %q", got)
	}
	// Lambda returning a non-string is coerced.
	if got := mustRender(t, "{{x}}", map[string]any{"x": Lambda(func(string) Value { return 7 })}, nil); got != "7" {
		t.Errorf("non-string lambda got %q", got)
	}
	// Section lambda returning a non-string.
	if got := mustRender(t, "{{#x}}b{{/x}}", map[string]any{"x": Lambda(func(string) Value { return 9 })}, nil); got != "9" {
		t.Errorf("non-string section lambda got %q", got)
	}
}

func TestGemAPI(t *testing.T) {
	m := &Mustache{
		Template: "{{greeting}}, {{>who}}!",
		Context:  map[string]any{"greeting": "Hi"},
		Partials: map[string]string{"who": "{{name}}"},
		View:     obj{m: map[string]Value{"name": "Ada"}},
	}
	got, err := m.Render()
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hi, Ada!" {
		t.Errorf("got %q", got)
	}
	// Context overrides a same-named View method (Context is above View).
	m2 := &Mustache{
		Template: "{{k}}",
		Context:  map[string]any{"k": "ctx"},
		View:     obj{m: map[string]Value{"k": "view"}},
	}
	if got, _ := m2.Render(); got != "ctx" {
		t.Errorf("precedence got %q", got)
	}
	// No View set.
	m3 := &Mustache{Template: "{{k}}", Context: map[string]any{"k": "v"}}
	if got, _ := m3.Render(); got != "v" {
		t.Errorf("no-view got %q", got)
	}
	// RenderString.
	if got, _ := RenderString("{{k}}", map[string]any{"k": "rs"}); got != "rs" {
		t.Errorf("RenderString got %q", got)
	}
}

func TestTypedEmptyLists(t *testing.T) {
	// Typed empty slices are treated as empty lists (falsey).
	for _, v := range []Value{[]string{}, []int{}} {
		if got := mustRender(t, "{{#x}}Y{{/x}}[{{^x}}N{{/x}}]", map[string]any{"x": v}, nil); got != "[N]" {
			t.Errorf("%T empty list got %q", v, got)
		}
	}
	// A non-empty typed slice is truthy but rendered once (not iterated).
	if got := mustRender(t, "{{#x}}Y{{/x}}", map[string]any{"x": []string{"a"}}, nil); got != "Y" {
		t.Errorf("typed non-empty got %q", got)
	}
}

func TestToStringFallback(t *testing.T) {
	// A shape outside the model falls back to Go formatting.
	type custom struct{ N int }
	if got := ToString(custom{N: 3}); !strings.Contains(got, "3") {
		t.Errorf("fallback got %q", got)
	}
	if ToString(nil) != "" {
		t.Errorf("nil to_s not empty")
	}
	// formatFloat's scientific / special branches.
	if got := ToString(1e300); !strings.Contains(got, "e+") {
		t.Errorf("exp float got %q", got)
	}
}

func TestErrors(t *testing.T) {
	bad := []string{
		"{{#x}}",       // unclosed section
		"{{x",          // unclosed tag
		"{{{x}}",       // unclosed triple
		"{{/x}}",       // stray close
		"{{#a}}{{/b}}", // mismatched close
		"{{=<%=}}",     // bad set-delimiter (one field)
		"{{=<% %>",     // unclosed set-delimiter
	}
	for _, tmpl := range bad {
		if _, err := Render(tmpl, nil, nil); err == nil {
			t.Errorf("expected error for %q", tmpl)
		}
	}
}

func TestLambdaBadTemplateError(t *testing.T) {
	// A lambda that returns an unparseable template surfaces the parse error.
	bad := Lambda(func(string) Value { return "{{#z}}" })
	if _, err := Render("{{x}}", map[string]any{"x": bad}, nil); err == nil {
		t.Error("expected interpolation-lambda parse error")
	}
	if _, err := Render("{{#x}}b{{/x}}", map[string]any{"x": Lambda(func(string) Value { return "{{#z}}" })}, nil); err == nil {
		t.Error("expected section-lambda parse error")
	}
	// Error propagation out of a list iteration (a nested lambda fails).
	ctx := map[string]any{
		"l": []any{map[string]any{"f": Lambda(func(string) Value { return "{{#z}}" })}},
	}
	if _, err := Render("{{#l}}{{f}}{{/l}}", ctx, nil); err == nil {
		t.Error("expected error propagated from list body")
	}
}

func TestPartialParseErrorPropagates(t *testing.T) {
	if _, err := Render("{{>p}}", nil, map[string]string{"p": "{{#z}}"}); err == nil {
		t.Error("expected partial parse error")
	}
}

func TestZeroValueMapSet(t *testing.T) {
	// A zero-value Map (not built via NewMap) lazily allocates its index on Set.
	var m Map
	m.Set("k", "v")
	if v, ok := m.Get("k"); !ok || v != "v" {
		t.Errorf("zero-value Map.Set/Get failed: %v %v", v, ok)
	}
}

func TestEmptyCloseTagAtTopLevel(t *testing.T) {
	// {{/}} closes an empty-named section; at top level (closeName "") it matches
	// and the parser stops with input remaining — the "unexpected close" guard.
	if _, err := Render("a{{/}}b", nil, nil); err == nil {
		t.Error("expected error for stray {{/}} at top level")
	}
}

func TestRawVarLambdaError(t *testing.T) {
	// A raw (unescaped) variable bound to a lambda returning a bad template.
	bad := Lambda(func(string) Value { return "{{#z}}" })
	if _, err := Render("{{{x}}}", map[string]any{"x": bad}, nil); err == nil {
		t.Error("expected raw-var lambda parse error")
	}
}

func TestLambdaResultRenderError(t *testing.T) {
	// x's lambda returns "{{y}}" (parses fine); y's lambda returns a bad template,
	// so the error surfaces from *rendering* x's result, not parsing it.
	ctx := map[string]any{
		"x": Lambda(func(string) Value { return "{{y}}" }),
		"y": Lambda(func(string) Value { return "{{#z}}" }),
	}
	if _, err := Render("{{x}}", ctx, nil); err == nil {
		t.Error("expected error rendering lambda result")
	}
}

func TestMustacheStructErrors(t *testing.T) {
	// Parse error in the Template.
	m := &Mustache{Template: "{{#x}}"}
	if _, err := m.Render(); err == nil {
		t.Error("expected Mustache.Render parse error")
	}
	// Render error via a lambda returning a bad template.
	m2 := &Mustache{
		Template: "{{x}}",
		Context:  map[string]any{"x": Lambda(func(string) Value { return "{{#z}}" })},
	}
	if _, err := m2.Render(); err == nil {
		t.Error("expected Mustache.Render render error")
	}
}

func TestPartialRecursionGuard(t *testing.T) {
	old := maxPartialDepth
	maxPartialDepth = 4
	defer func() { maxPartialDepth = old }()
	// A partial that always recurses (the section is always truthy) hits the guard.
	ctx := map[string]any{"go": true}
	_, err := Render("{{>p}}", ctx, map[string]string{"p": "{{#go}}{{>p}}{{/go}}"})
	if err == nil {
		t.Error("expected partial recursion-depth error")
	}
}
