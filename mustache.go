// Copyright (c) the go-ruby-mustache/mustache authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mustache

import "strings"

// Mustache is the gem's class-based view API. It pairs a template with a view —
// a set of named values (a Hash) and/or an [Object] exposing methods — plus a
// partial set, mirroring a `Mustache` subclass instance. Zero value is usable:
// set Template and (optionally) Context / View / Partials, then call Render.
type Mustache struct {
	// Template is the template source. When empty, Render renders nothing.
	Template string
	// Context is the primary view data — typically a map[string]any /
	// map[Symbol]any / *Map (a Ruby Hash). It is the bottom of the context stack.
	Context Value
	// View, when non-nil, is consulted for names not found in Context, modelling a
	// Mustache subclass's view methods. It sits below Context on the stack so
	// explicit data overrides a method of the same name (Ruby's precedence).
	View Object
	// Partials maps a partial name to its template source.
	Partials map[string]string
}

// Render renders the receiver's Template against its Context (and View, if set)
// and Partials — the instance form of `Mustache#render`.
func (m *Mustache) Render() (string, error) {
	nodes, err := parse(m.Template, defaultDelims)
	if err != nil {
		return "", err
	}
	r := &renderer{partials: m.Partials}
	if m.View != nil {
		r.stack = append(r.stack, m.View)
	}
	r.stack = append(r.stack, m.Context)
	var b strings.Builder
	if err := r.render(&b, nodes); err != nil {
		return "", err
	}
	return b.String(), nil
}

// RenderString renders template against context with no partials — the common
// one-shot form of the gem's `Mustache.render(template, context)`.
func RenderString(template string, context Value) (string, error) {
	return Render(template, context, nil)
}
