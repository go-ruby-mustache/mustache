// Copyright (c) the go-ruby-mustache/mustache authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mustache

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// The differential oracle renders a corpus through both this package and the
// system Ruby `mustache` gem and asserts byte-identical output. It is gated on
// Ruby ≥ 4.0 with the gem installed; where either is absent it skips itself, so
// the deterministic spec suite alone still drives the 100% coverage gate (the
// qemu cross-arch and Windows lanes have no gem). The corpus is limited to
// constructs on which the gem and the mustache spec agree (the gem 1.1.x differs
// from the spec on a few standalone-whitespace and dotted-name edge cases, which
// the embedded spec suite covers directly).

// rubyMustache locates a Ruby that can `require "mustache"` once, or skips.
func rubyMustache(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping mustache-gem oracle")
	}
	// Gate on Ruby ≥ 4.0 with the gem loadable; a non-zero exit skips the oracle.
	probe := `exit 1 unless RUBY_VERSION >= "4.0"; require "mustache"`
	if err := exec.Command(bin, "-e", probe).Run(); err != nil {
		t.Skip("ruby < 4.0 or mustache gem not installed; skipping oracle")
	}
	return bin
}

// gemRender renders template/data/partials with the Ruby mustache gem and
// returns its exact stdout bytes.
func gemRender(t *testing.T, bin string, template string, data any, partials map[string]string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"template": template,
		"data":     data,
		"partials": partials,
	})
	if err != nil {
		t.Fatalf("marshal oracle payload: %v", err)
	}
	// The script $stdout.binmode's so Windows text-mode cannot pollute the bytes
	// (the go-ruby-erb lesson), reads the JSON job on stdin, and prints the gem's
	// render. Partials are resolved from the supplied hash via a custom template.
	const script = `
$stdout.binmode
require "json"
require "mustache"
job = JSON.parse($stdin.read)
partials = job["partials"] || {}
klass = Class.new(Mustache) do
  define_method(:partial) { |name| partials[name.to_s] || "" }
end
view = klass.new
view.template = job["template"]
print view.render(job["template"], job["data"] || {})
`
	cmd := exec.Command(bin, "-e", script)
	cmd.Stdin = strings.NewReader(string(payload))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby mustache error: %v\noutput:\n%s", err, out)
	}
	return string(out)
}

// TestOracleDifferential renders a corpus through both engines and asserts the
// same bytes.
func TestOracleDifferential(t *testing.T) {
	bin := rubyMustache(t)

	cases := []struct {
		name     string
		tmpl     string
		data     any
		partials map[string]string
	}{
		{"plain", "just text", map[string]any{}, nil},
		{"var", "Hello, {{name}}!", map[string]any{"name": "world"}, nil},
		{"escape", "{{x}}", map[string]any{"x": `a & b < c > d " e ' f`}, nil},
		{"raw triple", "{{{x}}}", map[string]any{"x": `<b>`}, nil},
		{"raw amp", "{{&x}}", map[string]any{"x": `<b>`}, nil},
		{"section truthy", "{{#on}}yes{{/on}}", map[string]any{"on": true}, nil},
		{"section false", "[{{#on}}yes{{/on}}]", map[string]any{"on": false}, nil},
		{"section list", "{{#items}}[{{.}}]{{/items}}", map[string]any{"items": []any{"a", "b", "c"}}, nil},
		{"section hash", "{{#u}}{{name}}{{/u}}", map[string]any{"u": map[string]any{"name": "Ada"}}, nil},
		{"inverted absent", "{{^x}}none{{/x}}", map[string]any{}, nil},
		{"inverted empty", "{{^items}}none{{/items}}", map[string]any{"items": []any{}}, nil},
		{"comment", "a{{! ignored }}b", map[string]any{}, nil},
		{"nested", "{{#a}}{{#b}}{{c}}{{/b}}{{/a}}",
			map[string]any{"a": map[string]any{"b": map[string]any{"c": "deep"}}}, nil},
		{"list of hashes", "{{#rows}}<{{v}}>{{/rows}}",
			map[string]any{"rows": []any{map[string]any{"v": "1"}, map[string]any{"v": "2"}}}, nil},
		{"partial", "A{{>p}}B", map[string]any{"x": "mid"}, map[string]string{"p": "-{{x}}-"}},
		{"int", "{{n}}", map[string]any{"n": 42}, nil},
		{"multi vars", "{{a}}/{{b}}/{{c}}", map[string]any{"a": "1", "b": "2", "c": "3"}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := gemRender(t, bin, c.tmpl, c.data, c.partials)
			got, err := Render(c.tmpl, toModel(c.data), c.partials)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			if got != want {
				t.Errorf("differential mismatch\ntmpl: %q\n go: %q\ngem: %q", c.tmpl, got, want)
			}
		})
	}
}

// toModel converts the oracle's plain Go corpus data (built with int and nested
// maps/slices) into the package value model — here it is already in the model
// (map[string]any / []any / int / string / bool), so it passes through. It
// exists so the corpus and the JSON handed to Ruby share one source of truth.
func toModel(v any) Value { return v }
