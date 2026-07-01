// Copyright (c) the go-ruby-mustache/mustache authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mustache

import (
	"fmt"
	"strings"
)

// Render compiles template and renders it against context, resolving `{{>name}}`
// partials from the partials map (a nil or absent partial renders as the empty
// string). It is the package's core entry point, equivalent to the gem's
// `Mustache.render(template, context)` with an explicit partial set.
//
// context is a value from the package value model — typically a map[string]any /
// map[Symbol]any / *Map (a Ruby Hash), but any Value is accepted and pushed as
// the sole context frame. partials maps a partial name to its template source.
func Render(template string, context Value, partials map[string]string) (string, error) {
	nodes, err := parse(template, defaultDelims)
	if err != nil {
		return "", err
	}
	r := &renderer{partials: partials, stack: []Value{context}}
	var b strings.Builder
	if err := r.render(&b, nodes); err != nil {
		return "", err
	}
	return b.String(), nil
}

// renderer walks a node tree against a context stack.
type renderer struct {
	partials map[string]string
	stack    []Value // top of stack is the last element
	depth    int     // partial-recursion guard
}

// maxPartialDepth bounds partial recursion so a self-referential partial that
// never terminates (its section always truthy) fails cleanly instead of
// overflowing the stack. It is a var so tests can exercise the guard cheaply.
var maxPartialDepth = 100_000

// render writes the rendering of nodes to b.
func (r *renderer) render(b *strings.Builder, nodes []node) error {
	for i := range nodes {
		if err := r.renderNode(b, &nodes[i]); err != nil {
			return err
		}
	}
	return nil
}

// renderNode writes the rendering of a single node.
func (r *renderer) renderNode(b *strings.Builder, n *node) error {
	switch n.kind {
	case nodeText:
		b.WriteString(n.text)
	case nodeVar:
		v, err := r.interpolate(n.text)
		if err != nil {
			return err
		}
		b.WriteString(escapeHTML(v))
	case nodeRawVar:
		v, err := r.interpolate(n.text)
		if err != nil {
			return err
		}
		b.WriteString(v)
	case nodeSection:
		return r.renderSection(b, n)
	case nodeInverted:
		return r.renderInverted(b, n)
	case nodePartial:
		return r.renderPartial(b, n)
	}
	return nil
}

// interpolate resolves name to a string, invoking an interpolation lambda when
// the value is one.
func (r *renderer) interpolate(name string) (string, error) {
	v, ok := r.lookup(name)
	if !ok {
		return "", nil
	}
	if lam, isLam := asLambda(v); isLam {
		out := lam("")
		// An interpolation lambda's result is rendered against the default
		// delimiters, then interpolated.
		rendered, err := r.renderLambdaResult(out, defaultDelims)
		if err != nil {
			return "", err
		}
		return rendered, nil
	}
	return ToString(v), nil
}

// renderSection renders a {{#name}} … {{/name}} block.
func (r *renderer) renderSection(b *strings.Builder, n *node) error {
	v, ok := r.lookup(n.text)
	if !ok {
		return nil
	}

	// A lambda section: invoke with the raw body, render the result against the
	// section's delimiters.
	if lam, isLam := asLambda(v); isLam {
		out := lam(n.body)
		rendered, err := r.renderLambdaResult(out, n.delims)
		if err != nil {
			return err
		}
		b.WriteString(rendered)
		return nil
	}

	switch val := v.(type) {
	case nil:
		return nil
	case bool:
		if !val {
			return nil
		}
		return r.render(b, n.children)
	case []any:
		for _, item := range val {
			r.push(item)
			err := r.render(b, n.children)
			r.pop()
			if err != nil {
				return err
			}
		}
		return nil
	}

	if isEmptyList(v) {
		return nil
	}

	// Non-false, non-list value: render once with it atop the stack.
	r.push(v)
	err := r.render(b, n.children)
	r.pop()
	return err
}

// renderInverted renders a {{^name}} … {{/name}} block — its body renders only
// when name is absent, false, or an empty list.
func (r *renderer) renderInverted(b *strings.Builder, n *node) error {
	v, ok := r.lookup(n.text)
	if !ok {
		return r.render(b, n.children)
	}
	if truthy(v) {
		return nil
	}
	return r.render(b, n.children)
}

// renderPartial renders a {{>name}} partial, indenting every line by n.indent.
func (r *renderer) renderPartial(b *strings.Builder, n *node) error {
	src, ok := r.partials[n.text]
	if !ok || src == "" {
		return nil
	}
	if r.depth > maxPartialDepth {
		return fmt.Errorf("mustache: partial recursion too deep at %q", n.text)
	}
	if n.indent != "" {
		src = indentLines(src, n.indent)
	}
	nodes, err := parse(src, defaultDelims)
	if err != nil {
		return err
	}
	r.depth++
	err = r.render(b, nodes)
	r.depth--
	return err
}

// renderLambdaResult renders a lambda's returned value: a string result is
// parsed as a template (against delims) and rendered in the current context; any
// other value is coerced with ToString.
func (r *renderer) renderLambdaResult(v Value, delims delimiters) (string, error) {
	s, ok := v.(string)
	if !ok {
		return ToString(v), nil
	}
	nodes, err := parse(s, delims)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := r.render(&b, nodes); err != nil {
		return "", err
	}
	return b.String(), nil
}

// push/pop manage the context stack.
func (r *renderer) push(v Value) { r.stack = append(r.stack, v) }
func (r *renderer) pop()         { r.stack = r.stack[:len(r.stack)-1] }

// lookup resolves a Mustache name against the context stack. A bare "." is the
// value atop the stack (the implicit iterator). A dotted name resolves its first
// segment against the stack, then each further segment against that result.
func (r *renderer) lookup(name string) (Value, bool) {
	if name == "." {
		return r.stack[len(r.stack)-1], true
	}
	parts := strings.Split(name, ".")

	// Resolve the first segment by walking the stack top-to-bottom.
	first := parts[0]
	var cur Value
	found := false
	for i := len(r.stack) - 1; i >= 0; i-- {
		if v, ok := field(r.stack[i], first); ok {
			cur = v
			found = true
			break
		}
	}
	if !found {
		return nil, false
	}
	// Resolve the remaining segments against the successive results.
	for _, p := range parts[1:] {
		v, ok := field(cur, p)
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// field looks up a single name in one context value: a Hash key (string or
// Symbol) or an Object method. It reports whether the name was present.
func field(ctx Value, name string) (Value, bool) {
	switch c := ctx.(type) {
	case map[string]any:
		v, ok := c[name]
		return v, ok
	case map[Symbol]any:
		v, ok := c[Symbol(name)]
		return v, ok
	case *Map:
		if v, ok := c.Get(name); ok {
			return v, true
		}
		return c.Get(Symbol(name))
	case Object:
		return c.Method(name)
	}
	return nil, false
}

// truthy reports Mustache truthiness: nil and false and empty lists are falsey;
// everything else (including 0 and "") is truthy.
func truthy(v Value) bool {
	switch val := v.(type) {
	case nil:
		return false
	case bool:
		return val
	case []any:
		return len(val) > 0
	}
	if isEmptyList(v) {
		return false
	}
	return true
}

// isEmptyList reports whether v is an empty typed slice (e.g. []string{}). The
// []any case is handled by the callers before this is reached, so only the
// convenience typed-slice shapes are checked here.
func isEmptyList(v Value) bool {
	switch val := v.(type) {
	case []string:
		return len(val) == 0
	case []int:
		return len(val) == 0
	}
	return false
}

// asLambda adapts the accepted lambda shapes (Lambda, func() any,
// func(string) any) to a Lambda, reporting whether v was one.
func asLambda(v Value) (Lambda, bool) {
	switch f := v.(type) {
	case Lambda:
		return f, true
	case func(string) any:
		return Lambda(f), true
	case func() any:
		return func(string) any { return f() }, true
	}
	return nil, false
}

// escapeHTML escapes the five characters Ruby Mustache escapes (`CGI.escapeHTML`
// plus the single quote): & < > " '.
func escapeHTML(s string) string {
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// indentLines prefixes indent to the first line and every non-final line of src
// (a line that is empty and terminates the string is not indented — matching the
// spec's standalone-partial indentation, which does not pad a trailing blank).
func indentLines(src, indent string) string {
	var b strings.Builder
	b.Grow(len(src) + len(indent))
	b.WriteString(indent)
	for i := 0; i < len(src); i++ {
		b.WriteByte(src[i])
		if src[i] == '\n' && i != len(src)-1 {
			b.WriteString(indent)
		}
	}
	return b.String()
}

// defaultToString is the fallback used by ToString for shapes outside the
// documented model — Go's default formatting, matching a Ruby object's `to_s`
// only loosely (such values are not expected in Mustache data).
func defaultToString(v Value) string {
	return fmt.Sprintf("%v", v)
}
