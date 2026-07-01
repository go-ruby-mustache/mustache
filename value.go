// Copyright (c) the go-ruby-mustache/mustache authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package mustache is a pure-Go (CGO-free) implementation of Ruby's
// logic-less [Mustache] templating — the `mustache` gem's rendering engine —
// faithful to the language-independent [mustache spec]. It compiles a template
// to a small tree of tokens and renders it against a context drawn from the Ruby
// value model, so a host (such as go-embedded-ruby) can render Mustache
// templates against its own object graph without any Ruby runtime.
//
// # Ruby value model
//
// A context value is an [any] drawn from a small, fixed set of Go types so a
// host can map its own object graph to and from this package:
//
//	Ruby            Go
//	----            --
//	nil             nil
//	true / false    bool
//	Integer         int, int64, *big.Int
//	Float           float64, float32
//	String          string
//	Symbol          Symbol (a Hash key spelled :name)
//	Array           []any
//	Hash            map[string]any, map[Symbol]any, *Map (ordered)
//	Lambda / proc   Lambda, func() any, func(string) any
//	Object          Object (an interface exposing named methods)
//
// A Ruby Hash keyed by symbols is common in Mustache data; a plain
// map[string]any and a map[Symbol]any are both accepted and a name resolves
// against either. Everything else is coerced to a string with [ToString] before
// interpolation, matching Ruby's `to_s`.
//
// [Mustache]: https://mustache.github.io/
// [mustache spec]: https://github.com/mustache/spec
package mustache

import (
	"math/big"
	"strconv"
)

// Value is the interface satisfied by every context value this package handles.
// It is purely documentary — the public API uses any.
type Value = any

// Symbol is a Ruby Symbol (`:name`) used as a Hash key. A name lookup matches a
// Hash entry stored under either the plain string or the Symbol form, mirroring
// Ruby Mustache's tolerance of `{name: …}` data.
type Symbol string

// Lambda is a Mustache lambda. A section lambda receives the unrendered section
// body and returns a value rendered against the current delimiters; an
// interpolation lambda ignores the argument (Section is empty) and returns a
// value rendered against the default delimiters. Returning a string is the
// common case; any Value is accepted and coerced.
//
// The func() any and func(string) any Go shapes are also accepted directly as
// context values and adapted to this type, so a caller need not wrap a plain
// closure.
type Lambda func(section string) Value

// Object is a Ruby object exposing named "methods" to a template — a Mustache
// view. Method returns the value for the named method and whether the object
// responds to it. A host binds its own instances behind this interface so
// `{{name}}` and `{{#name}}` resolve to method calls.
type Object interface {
	Method(name string) (Value, bool)
}

// Pair is one entry of an ordered mapping.
type Pair struct {
	Key Value
	Val Value
}

// Map is an insertion-ordered Ruby Hash accepted as a context value. A name
// lookup matches a key stored as a string or a Symbol.
type Map struct {
	pairs []Pair
	index map[any]int
}

// NewMap returns an empty ordered Map.
func NewMap() *Map { return &Map{index: map[any]int{}} }

// Len reports the number of entries.
func (m *Map) Len() int { return len(m.pairs) }

// Pairs returns the entries in insertion order. The slice must not be mutated.
func (m *Map) Pairs() []Pair { return m.pairs }

// Set inserts or replaces the entry for key.
func (m *Map) Set(key, val Value) {
	if m.index == nil {
		m.index = map[any]int{}
	}
	if i, ok := m.index[key]; ok {
		m.pairs[i].Val = val
		return
	}
	m.index[key] = len(m.pairs)
	m.pairs = append(m.pairs, Pair{Key: key, Val: val})
}

// Get returns the value for key (string or Symbol) and whether it was present.
func (m *Map) Get(key Value) (Value, bool) {
	if i, ok := m.index[key]; ok {
		return m.pairs[i].Val, true
	}
	return nil, false
}

// ToString coerces a Value to its Ruby String form (`to_s`) for interpolation:
// nil is the empty string, a bool is "true"/"false", integers and floats use
// Ruby's formatting, a Symbol is its bare name, and a string is itself. Other
// shapes fall back to Go's default formatting.
func ToString(v Value) string {
	switch n := v.(type) {
	case nil:
		return ""
	case string:
		return n
	case Symbol:
		return string(n)
	case bool:
		if n {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case *big.Int:
		return n.String()
	case float64:
		return formatFloat(n)
	case float32:
		return formatFloat(float64(n))
	}
	return defaultToString(v)
}

// formatFloat renders a float the way Ruby's Float#to_s does for the values that
// occur in templates: an integral float keeps a trailing ".0".
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	// Ruby prints 2.0, not 2; add the ".0" when the g-form dropped it.
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == 'e' || s[i] == 'E' || s[i] == 'n' || s[i] == 'i' {
			return s
		}
	}
	return s + ".0"
}
