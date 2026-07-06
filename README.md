<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-mustache/brand/main/social/go-ruby-mustache-mustache.png" alt="go-ruby-mustache/mustache" width="720"></p>

# mustache — go-ruby-mustache

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-mustache.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) implementation of Ruby's logic-less [Mustache](https://mustache.github.io/)
templating** — the rendering engine of the [`mustache`](https://github.com/mustache/mustache)
gem, faithful to the language-independent
[mustache spec](https://github.com/mustache/spec). It compiles a template to a
small token tree and renders it against a context drawn from the Ruby value model,
so a host can render Mustache templates against its own object graph — **without
any Ruby runtime**.

It is the Mustache backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-erb](https://github.com/go-ruby-erb/erb) (the ERB compiler) and
[go-ruby-liquid](https://github.com/go-ruby-liquid/liquid).

## Features

Faithful to the mustache spec — **146/146** conformance examples pass across
`comments`, `interpolation`, `sections`, `inverted`, `partials`, `delimiters`
and `~lambdas`:

- **Variables** — `{{name}}` (HTML-escaped: `& < > " '`) and `{{{name}}}` /
  `{{&name}}` (unescaped).
- **Sections** — `{{#s}}…{{/s}}` over a truthy value, a list (iterating the
  body per item), an empty/false value (skipped), and a **lambda** (invoked with
  the raw body, its result re-rendered against the section's delimiters).
- **Inverted sections** — `{{^s}}…{{/s}}` render only when the name is absent,
  false, or an empty list.
- **Comments** `{{! … }}`, **partials** `{{> name}}` (with standalone
  indentation and recursion), and **set-delimiter** `{{=<% %>=}}`.
- **Dotted names** `{{a.b.c}}` with broken-chain resolution, the **implicit
  iterator** `{{.}}`, and a full **context-stack** lookup.
- **Lambdas** — interpolation lambdas (arity 0, rendered against the default
  delimiters) and section lambdas (arity 1, given the unprocessed body, rendered
  against the current delimiters), including stateful and delimiter-crossing
  cases.
- **Standalone lines** — a section / inverted / partial / comment / set-delimiter
  tag alone on its line strips the whole line (the fiddly whitespace rules).

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x).

## Install

```sh
go get github.com/go-ruby-mustache/mustache
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-mustache/mustache"
)

func main() {
	out, _ := mustache.Render(
		"Hello, {{name}}!\n{{#items}}- {{.}}\n{{/items}}",
		map[string]any{
			"name":  "world",
			"items": []any{"a", "b"},
		},
		nil, // no partials
	)
	fmt.Print(out)
	// Hello, world!
	// - a
	// - b
}
```

The gem's class-based view API:

```go
m := &mustache.Mustache{
	Template: `{{greeting}}, {{> who}}!`,
	Context:  map[string]any{"greeting": "Hi"},
	Partials: map[string]string{"who": "{{name}}"},
}
// A View (an mustache.Object) supplies method-style names too.
out, _ := m.Render()
```

## Ruby value model

A context value is an `any` drawn from a small, fixed set of Go types, so a host
can map its own object graph to and from this package:

| Ruby             | Go                                                    |
| ---------------- | ---------------------------------------------------- |
| `nil`            | `nil`                                                 |
| `true` / `false` | `bool`                                                |
| `Integer`        | `int`, `int64`, `*big.Int`                            |
| `Float`          | `float64`, `float32`                                  |
| `String`         | `string`                                              |
| `Symbol`         | `mustache.Symbol` (a Hash key spelled `:name`)        |
| `Array`          | `[]any`                                               |
| `Hash`           | `map[string]any`, `map[mustache.Symbol]any`, `*Map`   |
| lambda / `proc`  | `mustache.Lambda`, `func() any`, `func(string) any`   |
| object / view    | `mustache.Object` (exposes named methods)             |

A name resolves against a Hash key stored as either the string or the Symbol
form, and against an `Object`'s methods — the context-stack lookup the spec
describes.

## API

```go
// Render compiles template and renders it against context, resolving partials
// from the map (an absent partial renders as ""). Core entry point.
func Render(template string, context any, partials map[string]string) (string, error)

// RenderString is Render with no partials — Mustache.render(template, context).
func RenderString(template string, context any) (string, error)

// Mustache is the gem's class-based view API.
type Mustache struct {
	Template string
	Context  any
	View     Object            // consulted for names not in Context
	Partials map[string]string
}
func (m *Mustache) Render() (string, error)

type Symbol string
type Lambda func(section string) any
type Object interface{ Method(name string) (any, bool) }
type Map    struct { /* insertion-ordered Hash */ }
func NewMap() *Map
func (m *Map) Set(key, val any)
func (m *Map) Get(key any) (any, bool)

func ToString(v any) string // Ruby to_s for interpolation
```

## Tests & coverage

The suite embeds the mustache spec's JSON test files (`go:embed`) and asserts
**byte-identical** output for every example — a deterministic, ruby-free
conformance suite that alone holds coverage at **100%**, so the qemu cross-arch
and Windows CI lanes pass the gate. Lambda examples (whose spec data is a Ruby
`proc` source string) are backed by the equivalent Go lambda, keyed by test name.
A version-gated **differential MRI oracle** additionally renders a corpus with the
system `ruby` (`mustache` gem) and asserts the same bytes, where `ruby` is
present.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-mustache/mustache authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
