// Command csl-pattern validates a value against a named pattern in a CSL file.
// It can also convert a Go regular expression into a CSL pattern definition.
//
// Usage:
//   csl-pattern -file patterns.csl [-list] [-pattern Name] value
//   csl-pattern -regex-file expression.regex [-name PatternName]
// The -list option prints the names of all definitions in the CSL file.
// The -pattern option specifies which definition to use for validation.
// The -regex-file option reads a Go regular expression from a file and prints a
// corresponding CSL pattern definition.
// The -name option specifies the name of the generated pattern.
//
// The program exits with status 0 if the value is valid, 1 if invalid, and 2 for
// usage or compilation errors.

package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// tokenKind represents the type of a token in the CSL source.
type tokenKind int

const (
	tokEOF    tokenKind = iota // end of file
	tokIdent                   // identifier
	tokString                  // string literal
	tokLParen                  // left parenthesis
	tokRParen                  // right parenthesis
	tokLBrace                  // left brace
	tokRBrace                  // right brace
	tokEqual                   // equal sign
	tokPipe                    // pipe
	tokNumber                  // number
)

// token represents a lexical token in the CSL source.
type token struct {
	kind tokenKind // token category
	text string
	pos  int
}

// lexer scans CSL source and tracks its current byte position.
type lexer struct {
	src string
	pos int
}

// next reads the next token from the source, skipping whitespace.
func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) {
		r, n := utf8.DecodeRuneInString(l.src[l.pos:])
		if !unicode.IsSpace(r) {
			break
		}

		l.pos += n
	}

	if l.pos >= len(l.src) {
		return token{kind: tokEOF, pos: l.pos}, nil
	}

	start := l.pos
	r, n := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += n

	switch r {
	case '(':
		return token{kind: tokLParen, text: "(", pos: start}, nil
	case ')':
		return token{kind: tokRParen, text: ")", pos: start}, nil
	case '{':
		return token{kind: tokLBrace, text: "{", pos: start}, nil
	case '}':
		return token{kind: tokRBrace, text: "}", pos: start}, nil
	case '=':
		return token{kind: tokEqual, text: "=", pos: start}, nil
	case '|':
		return token{kind: tokPipe, text: "|", pos: start}, nil
	case '"':
		var b strings.Builder

		for l.pos < len(l.src) {
			ch, size := utf8.DecodeRuneInString(l.src[l.pos:])

			l.pos += size
			if ch == '"' {
				return token{kind: tokString, text: b.String(), pos: start}, nil
			}

			if ch == '\\' {
				if l.pos >= len(l.src) {
					return token{}, fmt.Errorf("position %d: unterminated escape sequence", l.pos)
				}

				escaped, escapedSize := utf8.DecodeRuneInString(l.src[l.pos:])
				l.pos += escapedSize

				switch escaped {
				case 'n':
					b.WriteRune('\n')
				case 'r':
					b.WriteRune('\r')
				case 't':
					b.WriteRune('\t')
				case '\\', '"':
					b.WriteRune(escaped)
				default:
					return token{}, fmt.Errorf("position %d: unsupported escape \\%c", l.pos-escapedSize, escaped)
				}

				continue
			}

			b.WriteRune(ch)
		}

		return token{}, fmt.Errorf("position %d: unterminated string literal", start)
	}

	if unicode.IsDigit(r) {
		for l.pos < len(l.src) {
			ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
			if !unicode.IsDigit(ch) {
				break
			}

			l.pos += size
		}

		return token{kind: tokNumber, text: l.src[start:l.pos], pos: start}, nil
	}

	if unicode.IsLetter(r) || r == '_' {
		for l.pos < len(l.src) {
			ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
			if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
				break
			}

			l.pos += size
		}

		return token{kind: tokIdent, text: l.src[start:l.pos], pos: start}, nil
	}

	return token{}, fmt.Errorf("position %d: unexpected character %q", start, r)
}

// nodeKind identifies the kind of expression represented by a node.
type nodeKind int

const (
	nodeLiteral     nodeKind = iota // quoted string literal
	nodeBuiltin                     // built-in character or pattern atom
	nodeSequence                    // concatenated child expressions
	nodeAlternative                 // alternative child expressions
	nodeRepeat                      // bounded or unbounded repetition
	nodeReference                   // named definition reference
)

// node is one parsed expression with child nodes or a reference target.
type node struct {
	id           int
	kind         nodeKind
	text         string
	literalRunes []rune
	children     []*node
	min          int
	max          int // -1 means unlimited
	target       *node
}

// definition stores one named type or pattern declaration.
type definition struct {
	name           string
	kind           string
	expr           *node
	pos            int
	strategy       matchStrategy
	nfa            *nfaProgram
	directRepeat   *fixedLiteralRepeat
	directChoice   *directLiteralChoice
	directSequence *directSequence
}

// parser tracks tokens, assigns node IDs, and stores parsed definitions.
type parser struct {
	tokens []token
	index  int
	nextID int
	defs   map[string]*definition
}

// lexAll tokenizes the complete source and includes its EOF token.
func lexAll(src string) ([]token, error) {
	l := lexer{src: src}

	var out []token

	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}

		out = append(out, t)
		if t.kind == tokEOF {
			return out, nil
		}
	}
}

// newParser tokenizes source and initializes an empty definition table.
func newParser(src string) (*parser, error) {
	tokens, err := lexAll(src)
	if err != nil {
		return nil, err
	}

	return &parser{tokens: tokens, defs: make(map[string]*definition)}, nil
}

// current returns the token at the parser cursor without advancing it.
func (p *parser) current() token {
	if p.index >= len(p.tokens) {
		return token{kind: tokEOF}
	}

	return p.tokens[p.index]
}

// take returns the current token and advances the parser cursor.
func (p *parser) take() token {
	t := p.current()
	if p.index < len(p.tokens) {
		p.index++
	}

	return t
}

// accept consumes the current token when its kind and optional text match.
func (p *parser) accept(kind tokenKind, text string) bool {
	t := p.current()
	if t.kind == kind && (text == "" || t.text == text) {
		p.take()
		return true
	}

	return false
}

// expect consumes a matching token or reports what was found instead.
func (p *parser) expect(kind tokenKind, text string) (token, error) {
	t := p.current()
	if t.kind != kind || (text != "" && t.text != text) {
		expected := text
		if expected == "" {
			expected = fmt.Sprintf("token kind %d", kind)
		}

		return token{}, fmt.Errorf("position %d: expected %s, got %q", t.pos, expected, t.text)
	}

	return p.take(), nil
}

// makeNode allocates a syntax node with a unique parser-local ID.
func (p *parser) makeNode(kind nodeKind, text string, children ...*node) *node {
	p.nextID++

	n := &node{id: p.nextID, kind: kind, text: text, children: children}
	if kind == nodeLiteral {
		n.literalRunes = []rune(text)
	}

	return n
}

// parse reads all definitions and resolves their references.
func (p *parser) parse() error {
	for p.current().kind != tokEOF {
		kw, err := p.expect(tokIdent, "")
		if err != nil {
			return err
		}

		if kw.text != "type" && kw.text != "pattern" {
			return fmt.Errorf("position %d: expected 'type' or 'pattern', got %q", kw.pos, kw.text)
		}

		name, err := p.expect(tokIdent, "")
		if err != nil {
			return err
		}

		if _, exists := p.defs[name.text]; exists {
			return fmt.Errorf("position %d: duplicate definition %q", name.pos, name.text)
		}

		d := &definition{name: name.text, kind: kw.text, pos: name.pos}
		p.defs[name.text] = d

		if kw.text == "type" {
			if _, err := p.expect(tokEqual, "="); err != nil {
				return err
			}

			d.expr, err = p.parseExpression()
			if err != nil {
				return err
			}
		} else {
			if _, err := p.expect(tokLBrace, "{"); err != nil {
				return err
			}

			d.expr, err = p.parseExpression()
			if err != nil {
				return err
			}

			if _, err := p.expect(tokRBrace, "}"); err != nil {
				return err
			}
		}
	}

	return p.resolveReferences()
}

// parseExpression reads one or more sequences separated by alternatives.
func (p *parser) parseExpression() (*node, error) {
	var alternatives []*node

	first, err := p.parseSequence()
	if err != nil {
		return nil, err
	}

	alternatives = append(alternatives, first)

	for p.accept(tokPipe, "|") {
		next, err := p.parseSequence()
		if err != nil {
			return nil, err
		}

		alternatives = append(alternatives, next)
	}

	if len(alternatives) == 1 {
		return alternatives[0], nil
	}

	return p.makeNode(nodeAlternative, "", alternatives...), nil
}

// parseSequence reads adjacent terms until an expression boundary.
func (p *parser) parseSequence() (*node, error) {
	var parts []*node

	for {
		t := p.current()
		if t.kind == tokEOF || t.kind == tokRParen || t.kind == tokRBrace || t.kind == tokPipe ||
			(t.kind == tokIdent && (t.text == "type" || t.text == "pattern")) {
			break
		}

		part, err := p.parseTerm()
		if err != nil {
			return nil, err
		}

		parts = append(parts, part)
	}

	if len(parts) == 0 {
		return nil, fmt.Errorf("position %d: expected pattern expression", p.current().pos)
	}

	if len(parts) == 1 {
		return parts[0], nil
	}

	return p.makeNode(nodeSequence, "", parts...), nil
}

// parseTerm reads an atom or a prefix repetition expression.
func (p *parser) parseTerm() (*node, error) {
	if p.current().kind == tokIdent && p.current().text == "repeat" {
		start := p.take()

		child, err := p.parseAtom()
		if err != nil {
			return nil, err
		}

		minToken, err := p.expect(tokNumber, "")
		if err != nil {
			return nil, fmt.Errorf("position %d: repeat requires a minimum count: %w", start.pos, err)
		}

		min, err := parseNonNegativeInt(minToken)
		if err != nil {
			return nil, err
		}

		if _, err := p.expect(tokIdent, "to"); err != nil {
			return nil, err
		}

		maxToken := p.current()
		max := -1

		if maxToken.kind == tokIdent && maxToken.text == "unlimited" {
			p.take()
		} else {
			if maxToken.kind != tokNumber {
				return nil, fmt.Errorf("position %d: expected maximum count or 'unlimited'", maxToken.pos)
			}

			p.take()

			max, err = parseNonNegativeInt(maxToken)
			if err != nil {
				return nil, err
			}
		}

		if max >= 0 && max < min {
			return nil, fmt.Errorf("position %d: repetition maximum %d is less than minimum %d", start.pos, max, min)
		}

		p.nextID++

		return &node{id: p.nextID, kind: nodeRepeat, children: []*node{child}, min: min, max: max}, nil
	}

	return p.parseAtom()
}

// parseNonNegativeInt converts a numeric token to a non-negative integer.
func parseNonNegativeInt(t token) (int, error) {
	var n int
	if _, err := fmt.Sscanf(t.text, "%d", &n); err != nil || n < 0 {
		return 0, fmt.Errorf("position %d: invalid non-negative integer %q", t.pos, t.text)
	}

	return n, nil
}

// parseAtom reads a literal, built-in, reference, optional, or grouped expression.
func (p *parser) parseAtom() (*node, error) {
	t := p.take()
	switch t.kind {
	case tokString:
		return p.makeNode(nodeLiteral, t.text), nil
	case tokIdent:
		if t.text == "optional" {
			child, err := p.parseAtom()
			if err != nil {
				return nil, err
			}

			p.nextID++

			return &node{id: p.nextID, kind: nodeRepeat, children: []*node{child}, min: 0, max: 1}, nil
		}

		if isBuiltin(t.text) {
			return p.makeNode(nodeBuiltin, t.text), nil
		}

		return p.makeNode(nodeReference, t.text), nil
	case tokLParen:
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}

		if _, err := p.expect(tokRParen, ")"); err != nil {
			return nil, err
		}

		return expr, nil
	default:
		return nil, fmt.Errorf("position %d: expected pattern term, got %q", t.pos, t.text)
	}
}

// isBuiltin reports whether name is a built-in character matcher.
func isBuiltin(name string) bool {
	switch name {
	case "digit", "digits", "ascii_digit", "ascii_digits", "letter", "letters", "lowercase_letter", "lowercase_letters", "uppercase_letter", "uppercase_letters", "ascii_letter", "ascii_letters", "whitespace", "whitespace_character", "any_character", "any_character_except_newline", "CamelCase", "PascalCase", "SnakeCase", "KebabCase", "URL":
		return true
	default:
		return false
	}
}

// resolveReferences links references and rejects unknown or recursive definitions.
func (p *parser) resolveReferences() error {
	for _, d := range p.defs {
		if err := p.resolveNode(d.expr); err != nil {
			return fmt.Errorf("definition %s: %w", d.name, err)
		}
	}

	state := make(map[string]int)

	var visit func(string) error

	visit = func(name string) error {
		if state[name] == 1 {
			return fmt.Errorf("recursive pattern definitions are not supported (cycle at %q)", name)
		}

		if state[name] == 2 {
			return nil
		}

		state[name] = 1

		d := p.defs[name]
		if d == nil {
			return fmt.Errorf("unknown definition %q", name)
		}

		var walk func(*node) error

		walk = func(n *node) error {
			if n.kind == nodeReference {
				if err := visit(n.text); err != nil {
					return err
				}
			}

			for _, c := range n.children {
				if err := walk(c); err != nil {
					return err
				}
			}

			return nil
		}
		if err := walk(d.expr); err != nil {
			return err
		}

		state[name] = 2

		return nil
	}
	for name := range p.defs {
		if err := visit(name); err != nil {
			return err
		}
	}

	return nil
}

// resolveNode links each reference below n to its target definition.
func (p *parser) resolveNode(n *node) error {
	if n.kind == nodeReference {
		d := p.defs[n.text]
		if d == nil {
			return fmt.Errorf("unknown pattern or type %q", n.text)
		}

		n.target = d.expr
	}

	for _, child := range n.children {
		if err := p.resolveNode(child); err != nil {
			return err
		}
	}

	return nil
}

// matchKey identifies a node and input position for memoized matching.
type matchKey struct {
	nodeID int
	pos    int
}

// matcher evaluates expression nodes against Unicode rune positions.
type matcher struct {
	input              []rune
	memo               map[matchKey][]int
	reuseBuffers       bool
	nfa                nfaWorkspace
	positionBuffers    [][]int
	positionBufferUsed int
	boolBuffers        [][]bool
	boolBufferUsed     int
}

// newMatcher creates a matcher for input with an empty memoization table.
func newMatcher(input string) *matcher {
	m := &matcher{memo: make(map[matchKey][]int)}
	m.setInput(input)

	return m
}

// reset prepares a reusable matcher for a new input without releasing its
// working-memory allocations. The matcher must not be in use by another call.
func (m *matcher) reset(input string) {
	m.setInput(input)
	clear(m.memo)
	m.positionBufferUsed = 0
	m.boolBufferUsed = 0
}

// retainable prevents a single unusually large input from permanently
// increasing the memory footprint of the shared matcher pool.
func (m *matcher) retainable() bool {
	if len(m.input) > 4096 || len(m.memo) > 8192 {
		return false
	}

	capacity := 0
	for _, buffer := range m.positionBuffers {
		capacity += cap(buffer)
		if capacity > 16384 {
			return false
		}
	}

	for _, buffer := range m.boolBuffers {
		capacity += cap(buffer)
		if capacity > 16384 {
			return false
		}
	}

	capacity += cap(m.nfa.active) + cap(m.nfa.next) + cap(m.nfa.stack) + cap(m.nfa.seen)
	if len(m.nfa.stateSets) > maxDFAStates || len(m.nfa.transitions) > maxDFATransitions || m.nfa.stateEntries > maxDFAStateSetEntries {
		return false
	}

	capacity += m.nfa.stateEntries
	if capacity > maxMatcherPoolScratch {
		return false
	}

	return true
}

func (m *matcher) setInput(input string) {
	m.input = m.input[:0]
	for len(input) > 0 {
		r, size := utf8.DecodeRuneInString(input)
		m.input = append(m.input, r)
		input = input[size:]
	}
}

// match returns every input position reachable by matching n from pos.
func (m *matcher) match(n *node, pos int) []int {
	if m.reuseBuffers {
		return m.matchReusable(n, pos)
	}

	return m.matchOneShot(n, pos)
}

func (m *matcher) matchReusable(n *node, pos int) []int {
	key := matchKey{nodeID: n.id, pos: pos}
	if cached, ok := m.memo[key]; ok {
		return cached
	}

	resultBuffer := m.takePositionBuffer(1)
	result := m.positionBuffers[resultBuffer]

	switch n.kind {
	case nodeLiteral:
		if pos+len(n.literalRunes) <= len(m.input) {
			matches := true

			for i, r := range n.literalRunes {
				if m.input[pos+i] != r {
					matches = false
					break
				}
			}

			if matches {
				result = append(result, pos+len(n.literalRunes))
			}
		}
	case nodeBuiltin:
		if isBuiltinPattern(n.text) {
			result = append(result, matchBuiltinPattern(n.text, m.input, pos)...)
		} else if pos < len(m.input) && builtinMatches(n.text, m.input[pos]) {
			result = append(result, pos+1)
		}
	case nodeReference:
		result = append(result, m.match(n.target, pos)...)
	case nodeAlternative:
		for _, child := range n.children {
			result = append(result, m.match(child, pos)...)
		}

		result = m.uniqueSortedPositions(result)
	case nodeSequence:
		positionsBuffer := m.takePositionBuffer(1)
		positions := append(m.positionBuffers[positionsBuffer], pos)

		m.positionBuffers[positionsBuffer] = positions
		for _, child := range n.children {
			nextBuffer := m.takePositionBuffer(len(positions))

			next := m.positionBuffers[nextBuffer]
			for _, current := range positions {
				next = append(next, m.match(child, current)...)
			}

			positions = m.uniqueSortedPositions(next)
			m.positionBuffers[nextBuffer] = positions
			positionsBuffer = nextBuffer

			if len(positions) == 0 {
				break
			}
		}

		result = append(result, positions...)
	case nodeRepeat:
		result = append(result, m.matchRepeatReusable(n, pos)...)
	}

	m.positionBuffers[resultBuffer] = result
	m.memo[key] = result

	return result
}

func (m *matcher) matchOneShot(n *node, pos int) []int {
	key := matchKey{nodeID: n.id, pos: pos}
	if cached, ok := m.memo[key]; ok {
		return cached
	}

	var result []int

	switch n.kind {
	case nodeLiteral:
		if pos+len(n.literalRunes) <= len(m.input) {
			matches := true

			for i, r := range n.literalRunes {
				if m.input[pos+i] != r {
					matches = false
					break
				}
			}

			if matches {
				result = []int{pos + len(n.literalRunes)}
			}
		}
	case nodeBuiltin:
		if isBuiltinPattern(n.text) {
			result = matchBuiltinPattern(n.text, m.input, pos)
		} else if pos < len(m.input) && builtinMatches(n.text, m.input[pos]) {
			result = []int{pos + 1}
		}
	case nodeReference:
		result = m.match(n.target, pos)
	case nodeAlternative:
		positions := make([]int, 0, len(n.children))
		for _, child := range n.children {
			positions = append(positions, m.match(child, pos)...)
		}

		result = m.uniqueSortedPositions(positions)
	case nodeSequence:
		positions := []int{pos}
		for _, child := range n.children {
			next := make([]int, 0, len(positions))
			for _, current := range positions {
				next = append(next, m.match(child, current)...)
			}

			positions = m.uniqueSortedPositions(next)
			if len(positions) == 0 {
				break
			}
		}

		result = positions
	case nodeRepeat:
		result = m.matchRepeatOneShot(n, pos)
	}

	m.memo[key] = result

	return result
}

func (m *matcher) matchRepeatOneShot(n *node, pos int) []int {
	child := n.children[0]
	if isSingleRuneMatcher(child) {
		maxCount := len(m.input) - pos
		if n.max >= 0 && n.max < maxCount {
			maxCount = n.max
		}

		matched := 0
		for matched < maxCount && matchesSingleRune(child, m.input[pos+matched]) {
			matched++
		}

		if matched < n.min {
			return nil
		}

		endpoints := make([]int, 0, matched-n.min+1)
		for count := n.min; count <= matched; count++ {
			endpoints = append(endpoints, pos+count)
		}

		return endpoints
	}

	frontier := []int{pos}

	accepted := make([]int, 0)
	if n.min == 0 {
		accepted = append(accepted, pos)
	}

	for count := 1; count <= n.min; count++ {
		frontier = m.advanceOneShot(child, frontier)
		if len(frontier) == 0 {
			return accepted
		}

		if count == n.min {
			accepted = append(accepted, frontier...)
		}
	}

	if n.max >= 0 {
		for count := n.min + 1; count <= n.max; count++ {
			frontier = m.advanceOneShot(child, frontier)
			if len(frontier) == 0 {
				break
			}

			accepted = append(accepted, frontier...)
		}

		return m.uniqueSortedPositions(accepted)
	}

	seen := make([]bool, len(m.input)+1)
	for _, current := range frontier {
		seen[current] = true
	}

	for {
		next := m.advanceOneShot(child, frontier)

		newFrontier := make([]int, 0, len(next))
		for _, current := range next {
			if !seen[current] {
				seen[current] = true
				newFrontier = append(newFrontier, current)
				accepted = append(accepted, current)
			}
		}

		if len(newFrontier) == 0 {
			break
		}

		frontier = newFrontier
	}

	return m.uniqueSortedPositions(accepted)
}

func (m *matcher) advanceOneShot(child *node, frontier []int) []int {
	next := make([]int, 0, len(frontier))
	for _, pos := range frontier {
		next = append(next, m.match(child, pos)...)
	}

	return m.uniqueSortedPositions(next)
}

// matchRepeat returns endpoints reached within the node's repetition bounds.
func (m *matcher) matchRepeatReusable(n *node, pos int) []int {
	child := n.children[0]
	if isSingleRuneMatcher(child) {
		return m.matchSingleRuneRepeat(n, pos)
	}

	frontierBuffer := m.takePositionBuffer(1)
	frontier := append(m.positionBuffers[frontierBuffer], pos)
	m.positionBuffers[frontierBuffer] = frontier
	acceptedBuffer := m.takePositionBuffer(0)
	accepted := m.positionBuffers[acceptedBuffer]

	if n.min == 0 {
		accepted = append(accepted, pos)
	}

	// First satisfy the minimum number of repetitions.
	for count := 1; count <= n.min; count++ {
		frontier = m.advance(child, frontier)
		if len(frontier) == 0 {
			m.positionBuffers[acceptedBuffer] = accepted
			return accepted
		}

		if count == n.min {
			accepted = append(accepted, frontier...)
		}
	}

	if n.max >= 0 {
		// Continue through the remaining permitted repetitions.
		for count := n.min + 1; count <= n.max; count++ {
			frontier = m.advance(child, frontier)
			if len(frontier) == 0 {
				break
			}

			accepted = append(accepted, frontier...)
		}

		accepted = m.uniqueSortedPositions(accepted)
		m.positionBuffers[acceptedBuffer] = accepted

		return accepted
	}

	// For an unlimited maximum, compute the transitive closure of reachable
	// endpoints. There are only len(input)+1 possible positions, so this ends
	// even if the repeated expression can match an empty string.
	seen := m.takeBoolBuffer(len(m.input) + 1)
	for _, pos := range frontier {
		seen[pos] = true
	}

	for {
		next := m.advance(child, frontier)
		newFrontierBuffer := m.takePositionBuffer(len(next))
		newFrontier := m.positionBuffers[newFrontierBuffer]

		for _, pos := range next {
			if !seen[pos] {
				seen[pos] = true
				newFrontier = append(newFrontier, pos)
				accepted = append(accepted, pos)
			}
		}

		if len(newFrontier) == 0 {
			break
		}

		m.positionBuffers[newFrontierBuffer] = newFrontier
		frontier = newFrontier
	}

	accepted = m.uniqueSortedPositions(accepted)
	m.positionBuffers[acceptedBuffer] = accepted

	return accepted
}

func isSingleRuneMatcher(n *node) bool {
	switch n.kind {
	case nodeLiteral:
		return len(n.literalRunes) == 1
	case nodeBuiltin:
		return !isBuiltinPattern(n.text)
	case nodeReference:
		return n.target != nil && isSingleRuneMatcher(n.target)
	case nodeAlternative:
		if len(n.children) == 0 {
			return false
		}

		for _, child := range n.children {
			if !isSingleRuneMatcher(child) {
				return false
			}
		}

		return true
	default:
		return false
	}
}

func matchesSingleRune(n *node, r rune) bool {
	switch n.kind {
	case nodeLiteral:
		return len(n.literalRunes) == 1 && n.literalRunes[0] == r
	case nodeBuiltin:
		return !isBuiltinPattern(n.text) && builtinMatches(n.text, r)
	case nodeReference:
		return n.target != nil && matchesSingleRune(n.target, r)
	case nodeAlternative:
		for _, child := range n.children {
			if matchesSingleRune(child, r) {
				return true
			}
		}
	}

	return false
}

func (m *matcher) matchSingleRuneRepeat(n *node, pos int) []int {
	maxCount := len(m.input) - pos
	if n.max >= 0 && n.max < maxCount {
		maxCount = n.max
	}

	matched := 0
	for matched < maxCount && matchesSingleRune(n.children[0], m.input[pos+matched]) {
		matched++
	}

	if matched < n.min {
		return nil
	}

	buffer := m.takePositionBuffer(matched - n.min + 1)

	endpoints := m.positionBuffers[buffer]
	for count := n.min; count <= matched; count++ {
		endpoints = append(endpoints, pos+count)
	}

	m.positionBuffers[buffer] = endpoints

	return endpoints
}

func (m *matcher) uniqueSortedPositions(positions []int) []int {
	if len(positions) < 2 {
		return positions
	}

	sort.Ints(positions)

	uniqueCount := 1
	for i := 1; i < len(positions); i++ {
		if positions[i] != positions[uniqueCount-1] {
			positions[uniqueCount] = positions[i]
			uniqueCount++
		}
	}

	return positions[:uniqueCount]
}

func (m *matcher) advance(child *node, frontier []int) []int {
	buffer := m.takePositionBuffer(len(frontier))

	next := m.positionBuffers[buffer]
	for _, pos := range frontier {
		next = append(next, m.match(child, pos)...)
	}

	next = m.uniqueSortedPositions(next)
	m.positionBuffers[buffer] = next

	return next
}

func (m *matcher) takePositionBuffer(minCapacity int) int {
	index := m.positionBufferUsed

	m.positionBufferUsed++
	if index == len(m.positionBuffers) {
		m.positionBuffers = append(m.positionBuffers, make([]int, 0, minCapacity))
	} else if cap(m.positionBuffers[index]) < minCapacity {
		m.positionBuffers[index] = make([]int, 0, minCapacity)
	} else {
		m.positionBuffers[index] = m.positionBuffers[index][:0]
	}

	return index
}

func (m *matcher) takeBoolBuffer(size int) []bool {
	index := m.boolBufferUsed

	m.boolBufferUsed++
	if index == len(m.boolBuffers) {
		m.boolBuffers = append(m.boolBuffers, make([]bool, size))
	} else if cap(m.boolBuffers[index]) < size {
		m.boolBuffers[index] = make([]bool, size)
	} else {
		m.boolBuffers[index] = m.boolBuffers[index][:size]
		clear(m.boolBuffers[index])
	}

	return m.boolBuffers[index]
}

// builtinMatches reports whether r satisfies the named built-in character atom.
func builtinMatches(name string, r rune) bool {
	switch name {
	case "digit", "digits":
		return unicode.IsDigit(r)
	case "ascii_digit", "ascii_digits":
		return r >= '0' && r <= '9'
	case "letter", "letters":
		return unicode.IsLetter(r)
	case "lowercase_letter", "lowercase_letters":
		return unicode.IsLower(r)
	case "uppercase_letter", "uppercase_letters":
		return unicode.IsUpper(r)
	case "ascii_letter", "ascii_letters":
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	case "whitespace", "whitespace_character":
		return unicode.IsSpace(r)
	case "any_character":
		return true
	case "any_character_except_newline":
		return r != '\n'
	default:
		return false
	}
}

// isBuiltinPattern reports whether name matches a multi-character built-in.
func isBuiltinPattern(name string) bool {
	switch name {
	case "CamelCase", "PascalCase", "SnakeCase", "KebabCase", "URL":
		return true
	default:
		return false
	}
}

// matchBuiltinPattern returns every endpoint that matches a multi-character built-in.
func matchBuiltinPattern(name string, input []rune, pos int) []int {
	if pos >= len(input) {
		return nil
	}

	switch name {
	case "CamelCase", "PascalCase":
		first := input[pos]
		if name == "CamelCase" && !unicode.IsLower(first) || name == "PascalCase" && !unicode.IsUpper(first) {
			return nil
		}

		var ends []int

		for end := pos + 1; end <= len(input); end++ {
			r := input[end-1]
			if end > pos+1 && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}

			ends = append(ends, end)
		}

		return ends
	case "SnakeCase", "KebabCase":
		separator := '_'
		if name == "KebabCase" {
			separator = '-'
		}

		var ends []int

		segmentStart := true

		for end := pos; end < len(input); end++ {
			r := input[end]
			if segmentStart {
				if !unicode.IsLower(r) {
					break
				}

				segmentStart = false

				ends = append(ends, end+1)

				continue
			}

			if unicode.IsLower(r) || unicode.IsDigit(r) {
				ends = append(ends, end+1)
				continue
			}

			if r != separator {
				break
			}

			segmentStart = true
		}

		return ends
	case "URL":
		var ends []int

		for end := pos + 1; end <= len(input); end++ {
			if unicode.IsSpace(input[end-1]) {
				break
			}

			if isHTTPURL(string(input[pos:end])) {
				ends = append(ends, end)
			}
		}

		return ends
	default:
		return nil
	}
}

// isHTTPURL reports whether value is an absolute HTTP or HTTPS URL with a host.
func isHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Hostname() == "" {
		return false
	}

	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}

// Program is a compiled set of named CSL type and pattern definitions.
type Program struct {
	definitions map[string]*definition
	matcherPool sync.Pool
}

// Matcher reuses its working memory across calls to Match. A Matcher is not
// safe for concurrent use; create one per goroutine when matching in parallel.
type Matcher struct {
	program *Program
	state   *matcher
}

// Compile parses source and returns a program whose definitions are resolved.
func Compile(source string) (*Program, error) {
	p, err := newParser(source)
	if err != nil {
		return nil, err
	}

	if err := p.parse(); err != nil {
		return nil, err
	}

	for _, definition := range p.defs {
		if choice, supported := compileDirectLiteralChoice(definition.expr); supported {
			definition.strategy = strategyDirect
			definition.directChoice = choice

			continue
		}

		if repeated, supported := compileFixedLiteralRepeat(definition.expr); supported {
			definition.strategy = strategyDirect
			definition.directRepeat = repeated

			continue
		}

		if sequence, supported := compileDirectSequence(definition.expr); supported {
			definition.strategy = strategyDirect
			definition.directSequence = sequence

			continue
		}

		if isDirectExpression(definition.expr, true) {
			definition.strategy = strategyDirect
			continue
		}

		if machine, supported := compileNFA(definition.expr); supported {
			definition.strategy = strategyNFA
			definition.nfa = machine

			continue
		}

		definition.strategy = strategyInterpreter
	}

	program := &Program{definitions: p.defs}
	program.matcherPool.New = func() any {
		return &matcher{memo: make(map[matchKey][]int), reuseBuffers: true}
	}

	return program, nil
}

// Names returns the program's definition names in sorted order.
func (p *Program) Names() []string {
	names := make([]string, 0, len(p.definitions))
	for name := range p.definitions {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// NewMatcher creates a matcher that reuses its rune and memoization storage.
func (p *Program) NewMatcher() *Matcher {
	return &Matcher{
		program: p,
		state:   &matcher{memo: make(map[matchKey][]int), reuseBuffers: true},
	}
}

// Match reports whether value is fully matched by the named definition.
func (p *Program) Match(name, value string) (bool, error) {
	d := p.definitions[name]
	if d == nil {
		return false, fmt.Errorf("unknown pattern %q", name)
	}

	switch d.strategy {
	case strategyDirect:
		if d.directSequence != nil {
			return d.directSequence.matches(value), nil
		}

		if d.directChoice != nil {
			return d.directChoice.matches(value), nil
		}

		if d.directRepeat != nil {
			return d.directRepeat.matches(value), nil
		}

		matched, _ := matchSimple(d.expr, value)

		return matched, nil
	case strategyNFA:
		m := p.matcherPool.Get().(*matcher)

		matched := m.nfa.match(d.nfa, value)
		if m.retainable() {
			p.matcherPool.Put(m)
		}

		return matched, nil
	}

	m := p.matcherPool.Get().(*matcher)
	m.reset(value)

	matched := m.matchDefinition(d)
	if m.retainable() {
		p.matcherPool.Put(m)
	}

	return matched, nil
}

// Match reports whether value is fully matched by a named definition.
func (m *Matcher) Match(name, value string) (bool, error) {
	if m == nil || m.program == nil || m.state == nil {
		return false, fmt.Errorf("matcher is not initialized")
	}

	d := m.program.definitions[name]
	if d == nil {
		return false, fmt.Errorf("unknown pattern %q", name)
	}

	switch d.strategy {
	case strategyDirect:
		if d.directSequence != nil {
			return d.directSequence.matches(value), nil
		}

		if d.directChoice != nil {
			return d.directChoice.matches(value), nil
		}

		if d.directRepeat != nil {
			return d.directRepeat.matches(value), nil
		}

		matched, _ := matchSimple(d.expr, value)

		return matched, nil
	case strategyNFA:
		return m.state.nfa.match(d.nfa, value), nil
	}

	m.state.reset(value)

	return m.state.matchDefinition(d), nil
}

func (m *matcher) matchDefinition(d *definition) bool {
	for _, end := range m.match(d.expr, 0) {
		if end == len(m.input) {
			return true
		}
	}

	return false
}

func matchSimple(n *node, input string) (bool, bool) {
	root := n
	for root.kind == nodeReference && root.target != nil {
		root = root.target
	}

	if root.kind == nodeAlternative {
		for _, branch := range root.children {
			pos := 0

			matched, supported := matchSimpleNode(branch, input, &pos)
			if !supported {
				return false, false
			}

			if matched && pos == len(input) {
				return true, true
			}
		}

		return false, true
	}

	if root.kind == nodeRepeat && isSingleRuneMatcher(root.children[0]) {
		count := 0

		for len(input) > 0 {
			r, size := utf8.DecodeRuneInString(input)
			if !matchesSingleRune(root.children[0], r) || root.max >= 0 && count == root.max {
				return false, true
			}

			count++
			input = input[size:]
		}

		return count >= root.min, true
	}

	pos := 0
	matched, supported := matchSimpleNode(n, input, &pos)

	return matched && pos == len(input), supported
}

func matchSimpleNode(n *node, input string, pos *int) (bool, bool) {
	switch n.kind {
	case nodeLiteral:
		for _, expected := range n.literalRunes {
			if *pos >= len(input) {
				return false, true
			}

			actual, size := utf8.DecodeRuneInString(input[*pos:])
			if actual != expected {
				return false, true
			}

			*pos += size
		}

		return true, true
	case nodeBuiltin:
		if isBuiltinPattern(n.text) || *pos >= len(input) {
			return false, !isBuiltinPattern(n.text)
		}

		r, size := utf8.DecodeRuneInString(input[*pos:])
		if !builtinMatches(n.text, r) {
			return false, true
		}

		*pos += size

		return true, true
	case nodeReference:
		return matchSimpleNode(n.target, input, pos)
	case nodeSequence:
		for _, child := range n.children {
			matched, supported := matchSimpleNode(child, input, pos)
			if !supported || !matched {
				return matched, supported
			}
		}

		return true, true
	default:
		return false, false
	}
}

// convertRegexFile translates a regex file into a validated named CSL definition.
func convertRegexFile(path, name string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	regex := strings.TrimRight(string(content), "\r\n")

	expression, err := RegexToPattern(regex)
	if err != nil {
		return "", err
	}

	if name == "" {
		name = "RegexPattern"
	}

	lines := strings.Split(expression, "\n")
	for i := range lines {
		lines[i] = "    " + lines[i]
	}

	source := fmt.Sprintf("pattern %s {\n%s\n}\n", name, strings.Join(lines, "\n"))
	if _, err := Compile(source); err != nil {
		return "", fmt.Errorf("generated invalid CSL pattern: %w", err)
	}

	return source, nil
}

// main parses command-line options and runs the requested pattern operation.
func main() {
	file := flag.String("file", "", "pattern definition file")
	regexFile := flag.String("regex-file", "", "Go regular expression file to convert to CSL")
	name := flag.String("name", "", "name for the generated pattern (regex conversion only)")
	pattern := flag.String("pattern", "", "definition to test")
	list := flag.Bool("list", false, "list available definitions")

	flag.Parse()

	if *regexFile != "" {
		if *file != "" || *pattern != "" || *list || flag.NArg() != 0 {
			fatal(fmt.Errorf("-regex-file cannot be combined with -file, -pattern, -list, or positional values"))
		}

		generated, err := convertRegexFile(*regexFile, *name)
		if err != nil {
			fatal(err)
		}

		fmt.Print(generated)

		return
	}

	if *name != "" {
		fatal(fmt.Errorf("-name can only be used with -regex-file"))
	}

	if *file == "" {
		fmt.Fprintln(os.Stderr, "usage: csl-pattern -file patterns.csl [-list] [-pattern Name] value")
		fmt.Fprintln(os.Stderr, "       csl-pattern -regex-file expression.regex [-name PatternName]")
		os.Exit(2)
	}

	source, err := os.ReadFile(*file)
	if err != nil {
		fatal(err)
	}

	program, err := Compile(string(source))
	if err != nil {
		fatal(err)
	}

	if *list {
		for _, name := range program.Names() {
			fmt.Println(name)
		}

		return
	}

	if *pattern == "" {
		fatal(fmt.Errorf("-pattern is required unless -list is used"))
	}

	if flag.NArg() != 1 {
		fatal(fmt.Errorf("expected exactly one value argument"))
	}

	ok, err := program.Match(*pattern, flag.Arg(0))
	if err != nil {
		fatal(err)
	}

	if ok {
		fmt.Println("valid")
		return
	}

	fmt.Println("invalid")
	os.Exit(1)
}

// fatal prints an error and exits with the usage or compilation status.
func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(2)
}
