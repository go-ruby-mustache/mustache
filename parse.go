// Copyright (c) the go-ruby-mustache/mustache authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mustache

import (
	"fmt"
	"strings"
)

// nodeKind enumerates the token shapes the parser emits.
type nodeKind int

const (
	nodeText     nodeKind = iota // literal text
	nodeVar                      // {{name}} — HTML-escaped interpolation
	nodeRawVar                   // {{{name}}} or {{&name}} — unescaped
	nodeSection                  // {{#name}} … {{/name}}
	nodeInverted                 // {{^name}} … {{/name}}
	nodePartial                  // {{>name}}
)

// node is one element of a compiled template. For a section/inverted node,
// children holds the parsed body, body is the raw template source of the body
// (for section lambdas), and delims records the delimiters in force inside the
// section (so a section-body lambda re-renders with them).
type node struct {
	kind     nodeKind
	text     string // literal text (nodeText) or the tag name (otherwise)
	children []node
	body     string     // raw source of a section body (lambda re-render)
	delims   delimiters // delimiters active inside a section
	indent   string     // standalone indentation of a partial
}

// delimiters is the active open/close delimiter pair.
type delimiters struct {
	open, close string
}

var defaultDelims = delimiters{open: "{{", close: "}}"}

// parse compiles a template into a node tree with the given starting delimiters.
func parse(template string, delims delimiters) ([]node, error) {
	p := &parser{src: template, delims: delims, lineStart: true}
	nodes, _, err := p.parseNodes("")
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("mustache: unexpected close of section")
	}
	return nodes, nil
}

// parser holds the scan state.
type parser struct {
	src       string
	pos       int
	delims    delimiters
	lineStart bool // true when p.pos is at the start of a line (col 0)
}

// rawTag is one scanned tag.
type rawTag struct {
	sigil   byte   // 0 for a plain variable, else # ^ / > & { ! =
	name    string // trimmed tag content
	content string // full inner content (for set-delimiter parsing)
	end     int    // index in src just past the trailing close delimiter
}

// parseNodes parses tokens until end-of-input or a closing tag matching
// closeName (when non-empty). It returns the parsed nodes and, when it stopped
// on a matching close tag, the raw source consumed for the body (used by section
// lambdas). It performs standalone-line stripping.
func (p *parser) parseNodes(closeName string) ([]node, string, error) {
	var nodes []node
	bodyStart := p.pos
	for p.pos < len(p.src) {
		idx := strings.Index(p.src[p.pos:], p.delims.open)
		if idx < 0 {
			nodes = p.appendText(nodes, p.src[p.pos:])
			p.pos = len(p.src)
			break
		}
		tagStart := p.pos + idx
		leading := p.src[p.pos:tagStart]

		tag, err := p.scanTag(tagStart)
		if err != nil {
			return nil, "", err
		}

		standalone, trailConsumed := p.standalone(tag, leading, tagStart)

		// Text before the tag; a standalone tag drops its line's leading blanks.
		emitLead := leading
		if standalone {
			emitLead = leading[:lastLineStart(leading)]
		}
		nodes = p.appendText(nodes, emitLead)

		// Advance past the tag (and, if standalone, its trailing line blanks +
		// newline). After a standalone tag we are at the start of a new line.
		p.pos = tag.end
		if standalone {
			p.pos += trailConsumed
			p.lineStart = true
		} else {
			p.lineStart = false
		}

		switch tag.sigil {
		case '!':
			// comment — nothing emitted.
		case '=':
			if err := p.setDelimiters(tag.content); err != nil {
				return nil, "", err
			}
		case '#', '^':
			inner, err := p.parseSection(tag)
			if err != nil {
				return nil, "", err
			}
			nodes = append(nodes, inner)
		case '/':
			if tag.name != closeName {
				return nil, "", fmt.Errorf("mustache: unexpected close tag {{/%s}}", tag.name)
			}
			return nodes, p.src[bodyStart:tagStart], nil
		case '>':
			ind := ""
			if standalone {
				ind = leading[lastLineStart(leading):]
			}
			nodes = append(nodes, node{kind: nodePartial, text: tag.name, indent: ind})
		case '{', '&':
			nodes = append(nodes, node{kind: nodeRawVar, text: tag.name})
		default:
			nodes = append(nodes, node{kind: nodeVar, text: tag.name})
		}
	}
	if closeName != "" {
		return nil, "", fmt.Errorf("mustache: unclosed section {{#%s}}", closeName)
	}
	return nodes, "", nil
}

// parseSection parses the body of a #/^ section up to its matching close tag.
func (p *parser) parseSection(tag rawTag) (node, error) {
	kind := nodeSection
	if tag.sigil == '^' {
		kind = nodeInverted
	}
	openDelims := p.delims
	children, body, err := p.parseNodes(tag.name)
	if err != nil {
		return node{}, err
	}
	return node{kind: kind, text: tag.name, children: children, body: body, delims: openDelims}, nil
}

// scanTag reads a single tag beginning at start (which indexes the open
// delimiter) and returns its decoded form.
func (p *parser) scanTag(start int) (rawTag, error) {
	openLen := len(p.delims.open)
	inner := start + openLen

	// A triple-mustache {{{ … }}} only applies to the default {{ }} pair.
	if p.delims.open == "{{" && p.delims.close == "}}" && strings.HasPrefix(p.src[inner:], "{") {
		const close = "}}}"
		ci := strings.Index(p.src[inner+1:], close)
		if ci < 0 {
			return rawTag{}, fmt.Errorf("mustache: unclosed triple mustache")
		}
		body := p.src[inner+1 : inner+1+ci]
		return rawTag{sigil: '{', name: strings.TrimSpace(body), content: body, end: inner + 1 + ci + len(close)}, nil
	}

	rest := p.src[inner:]
	closeDelim := p.delims.close

	// Set-delimiter tags carry a trailing '=' before the close delimiter.
	if strings.HasPrefix(rest, "=") {
		end := strings.Index(rest, "="+closeDelim)
		if end < 0 {
			return rawTag{}, fmt.Errorf("mustache: unclosed set-delimiter tag")
		}
		return rawTag{sigil: '=', content: rest[1:end], end: inner + end + 1 + len(closeDelim)}, nil
	}

	ci := strings.Index(rest, closeDelim)
	if ci < 0 {
		return rawTag{}, fmt.Errorf("mustache: unclosed tag")
	}
	body := rest[:ci]
	end := inner + ci + len(closeDelim)

	trimmed := strings.TrimSpace(body)
	var sigil byte
	if trimmed != "" {
		switch trimmed[0] {
		case '#', '^', '/', '>', '&', '!':
			sigil = trimmed[0]
			trimmed = strings.TrimSpace(trimmed[1:])
		}
	}
	return rawTag{sigil: sigil, name: trimmed, content: body, end: end}, nil
}

// setDelimiters applies a {{= open close =}} directive.
func (p *parser) setDelimiters(content string) error {
	fields := strings.Fields(content)
	if len(fields) != 2 {
		return fmt.Errorf("mustache: invalid set-delimiter %q", content)
	}
	p.delims = delimiters{open: fields[0], close: fields[1]}
	return nil
}

// standalone reports whether tag is a standalone tag — the only non-whitespace
// content on its line — and how many trailing bytes (line blanks + newline) to
// consume after the tag. Only section, inverted, close, partial, comment and
// set-delimiter tags may be standalone; plain and raw variables never are.
func (p *parser) standalone(tag rawTag, leading string, tagStart int) (bool, int) {
	switch tag.sigil {
	case '#', '^', '/', '>', '!', '=':
	default:
		return false, 0
	}

	// Left: the text since the previous tag, from its last newline onward, must
	// be all blanks; and the tag must sit at a line start (either that leading
	// text contains a newline, or the parser was already at a line start).
	preLine := leading[lastLineStart(leading):]
	if strings.TrimLeft(preLine, " \t") != "" {
		return false, 0
	}
	if !strings.Contains(leading, "\n") && !p.lineStart {
		return false, 0
	}

	// Right: from just after the tag up to and including the next newline (or
	// end of input) must be all blanks.
	after := p.src[tag.end:]
	i := 0
	for i < len(after) && (after[i] == ' ' || after[i] == '\t') {
		i++
	}
	switch {
	case i == len(after):
		return true, i
	case after[i] == '\n':
		return true, i + 1
	case after[i] == '\r' && i+1 < len(after) && after[i+1] == '\n':
		return true, i + 2
	}
	return false, 0
}

// lastLineStart returns the index just after the last newline in s (0 if none).
func lastLineStart(s string) int {
	return strings.LastIndexByte(s, '\n') + 1
}

// appendText appends literal text, coalescing with a trailing text node, and
// updates the line-start flag from the emitted bytes.
func (p *parser) appendText(nodes []node, text string) []node {
	if text == "" {
		return nodes
	}
	p.lineStart = text[len(text)-1] == '\n'
	if n := len(nodes); n > 0 && nodes[n-1].kind == nodeText {
		nodes[n-1].text += text
		return nodes
	}
	return append(nodes, node{kind: nodeText, text: text})
}
