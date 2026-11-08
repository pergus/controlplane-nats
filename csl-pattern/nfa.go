// Package main implements non-deterministic finite automata for pattern
// matching. It provides a way to compile patterns into NFA programs and match
// input strings against them. The NFA implementation supports various pattern
// constructs, including literals, built-in patterns, sequences, alternatives,
// and repetitions. The NFA can be used directly for matching or can be
// converted into a DFA for more efficient matching.
//
// The NFA implementation includes optimizations for direct literal choices and
// fixed literal repeats, allowing for faster matching in specific cases. The
// NFA workspace manages the active states during matching and provides caching
// mechanisms to improve performance when matching multiple inputs against the
// same NFA program.

package main

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// These limits bound NFA and DFA construction and caching costs.
const (
	// maxNFAStates is the maximum number of states in a compiled NFA.
	maxNFAStates = 8192
	// maxDFAStates is the maximum number of cached DFA states per workspace.
	maxDFAStates = 1024
	// maxDFATransitions is the maximum number of cached DFA transitions.
	maxDFATransitions = 8192
	// maxDFAStateSetEntries limits the total states stored across cached sets.
	maxDFAStateSetEntries = 65536
	// maxMatcherPoolScratch limits scratch storage retained by matcher pools.
	maxMatcherPoolScratch = 81920
	// maxLiteralAlternatives bounds literal expansion during compilation.
	maxLiteralAlternatives = 256
)

// matchStrategy identifies the execution path used to match a pattern.
type matchStrategy uint8

const (
	// strategyDirect matches patterns with a specialized direct matcher.
	strategyDirect matchStrategy = iota
	// strategyNFA matches patterns using a compiled NFA.
	strategyNFA
	// strategyInterpreter matches patterns with the general interpreter.
	strategyInterpreter
)

// nfaTransition describes a literal or built-in transition to another state.
type nfaTransition struct {
	target  int
	literal rune
	builtin string
}

// nfaState stores outgoing transitions and epsilon edges in an NFA.
type nfaState struct {
	epsilon            []int
	literalTransitions []nfaTransition
	builtinTransitions []nfaTransition
}

// nfaProgram contains the states and endpoints of a compiled NFA.
type nfaProgram struct {
	states []nfaState
	start  int
	accept int
}

// nfaFragment identifies the entry and exit states of a compiled expression.
type nfaFragment struct {
	start  int
	accept int
}

// nfaBuilder constructs NFA states and connects expression fragments.
type nfaBuilder struct {
	states []nfaState
}

// fixedLiteralRepeat stores equal-width literal choices and their repeat bounds.
type fixedLiteralRepeat struct {
	words [][]rune
	min   int
	max   int
}

// directLiteralChoice stores alternatives as a shared prefix and suffixes.
type directLiteralChoice struct {
	prefix     string
	suffixes   []string
	prefixFree bool
}

// directSequencePart represents one literal, built-in, choice, or repeat part.
type directSequencePart struct {
	literal     []rune
	builtin     string
	choice      *directLiteralChoice
	repeatChild *node
	repeatMin   int
	repeatMax   int
}

// directSequence is a specialized matcher plan for supported sequences.
type directSequence struct {
	parts []directSequencePart
}

// compileDirectLiteralChoice compiles a literal-only alternative for direct matching.
func compileDirectLiteralChoice(root *node) (*directLiteralChoice, bool) {
	for root.kind == nodeReference && root.target != nil {
		root = root.target
	}

	if root.kind != nodeAlternative {
		return nil, false
	}

	words, ok := collectLiteralWords(root)
	if !ok || len(words) < 2 {
		return nil, false
	}

	prefixLength := len(words[0])
	for _, word := range words[1:] {
		if len(word) < prefixLength {
			prefixLength = len(word)
		}

		for i := 0; i < prefixLength; i++ {
			if word[i] != words[0][i] {
				prefixLength = i
				break
			}
		}
	}

	choice := &directLiteralChoice{
		prefix:     string(words[0][:prefixLength]),
		suffixes:   make([]string, len(words)),
		prefixFree: true,
	}
	for i, word := range words {
		choice.suffixes[i] = string(word[prefixLength:])
	}

	for i, suffix := range choice.suffixes {
		for j, other := range choice.suffixes {
			if i != j && len(suffix) < len(other) && strings.HasPrefix(other, suffix) {
				choice.prefixFree = false
				break
			}
		}
	}

	return choice, true
}

// matches reports whether input is exactly one of the compiled alternatives.
func (c *directLiteralChoice) matches(input string) bool {
	if !strings.HasPrefix(input, c.prefix) {
		return false
	}

	suffix := input[len(c.prefix):]
	for _, candidate := range c.suffixes {
		if suffix == candidate {
			return true
		}
	}

	return false
}

// matchPrefix returns the length of the first matching alternative prefix.
func (c *directLiteralChoice) matchPrefix(input string) (int, bool) {
	if !strings.HasPrefix(input, c.prefix) {
		return 0, false
	}

	remainder := input[len(c.prefix):]
	for _, suffix := range c.suffixes {
		if strings.HasPrefix(remainder, suffix) {
			return len(c.prefix) + len(suffix), true
		}
	}

	return 0, false
}

// compileDirectSequence builds a direct matcher plan when the sequence supports it.
func compileDirectSequence(root *node) (*directSequence, bool) {
	plan := &directSequence{}

	hasSpecial := false
	if !appendDirectSequenceParts(root, plan, &hasSpecial) || !hasSpecial || len(plan.parts) > 128 {
		return nil, false
	}

	return plan, true
}

// appendDirectSequenceParts flattens supported nodes into a direct matcher plan.
func appendDirectSequenceParts(n *node, plan *directSequence, hasSpecial *bool) bool {
	switch n.kind {
	case nodeReference:
		return n.target != nil && appendDirectSequenceParts(n.target, plan, hasSpecial)
	case nodeSequence:
		for _, child := range n.children {
			if !appendDirectSequenceParts(child, plan, hasSpecial) {
				return false
			}
		}

		return true
	case nodeLiteral:
		plan.parts = append(plan.parts, directSequencePart{literal: n.literalRunes})
		return true
	case nodeBuiltin:
		if isBuiltinPattern(n.text) {
			return false
		}

		plan.parts = append(plan.parts, directSequencePart{builtin: n.text})

		return true
	case nodeAlternative:
		choice, ok := compileDirectLiteralChoice(n)
		if !ok {
			return false
		}

		*hasSpecial = true

		plan.parts = append(plan.parts, directSequencePart{choice: choice})

		return true
	case nodeRepeat:
		if !isSingleRuneMatcher(n.children[0]) {
			return false
		}

		*hasSpecial = true

		plan.parts = append(plan.parts, directSequencePart{
			repeatChild: n.children[0],
			repeatMin:   n.min,
			repeatMax:   n.max,
		})

		return true
	default:
		return false
	}
}

// matches reports whether input matches the complete direct sequence.
func (p *directSequence) matches(input string) bool {
	return p.matchesFrom(0, input)
}

// matchesFrom matches the sequence suffix beginning at index against input.
func (p *directSequence) matchesFrom(index int, input string) bool {
	if index == len(p.parts) {
		return len(input) == 0
	}

	part := p.parts[index]
	if part.choice != nil {
		if !strings.HasPrefix(input, part.choice.prefix) {
			return false
		}

		remainder := input[len(part.choice.prefix):]
		for _, suffix := range part.choice.suffixes {
			if strings.HasPrefix(remainder, suffix) && p.matchesFrom(index+1, remainder[len(suffix):]) {
				return true
			}
		}

		return false
	}

	if part.repeatChild != nil {
		maxCount := len(input)
		if part.repeatMax >= 0 && part.repeatMax < maxCount {
			maxCount = part.repeatMax
		}

		count, bytesConsumed := 0, 0
		for count < maxCount && bytesConsumed < len(input) {
			r, size := utf8.DecodeRuneInString(input[bytesConsumed:])
			if !matchesSingleRune(part.repeatChild, r) {
				break
			}

			count++
			bytesConsumed += size
		}

		for count >= part.repeatMin {
			if p.matchesFrom(index+1, input[bytesConsumed:]) {
				return true
			}

			if count == part.repeatMin {
				break
			}

			_, size := utf8.DecodeLastRuneInString(input[:bytesConsumed])
			bytesConsumed -= size
			count--
		}

		return false
	}

	if part.builtin != "" {
		if len(input) == 0 {
			return false
		}

		r, size := utf8.DecodeRuneInString(input)

		return builtinMatches(part.builtin, r) && p.matchesFrom(index+1, input[size:])
	}

	for _, expected := range part.literal {
		if len(input) == 0 {
			return false
		}

		actual, size := utf8.DecodeRuneInString(input)
		if actual != expected {
			return false
		}

		input = input[size:]
	}

	return p.matchesFrom(index+1, input)
}

// nfaWorkspace reuses scratch buffers and caches DFA states while matching.
type nfaWorkspace struct {
	active       []int
	next         []int
	stack        []int
	seen         []uint32
	generation   uint32
	program      *nfaProgram
	stateSets    [][]int
	stateIDs     map[string]int
	transitions  map[dfaTransitionKey]int
	stateEntries int
	cacheFull    bool
	startState   int
	startCached  bool
	stats        *nfaStats
}

// nfaStats records NFA and DFA work performed during a match.
type nfaStats struct {
	closureVisits     int
	activeStateVisits int
	transitionChecks  int
	dfaHits           int
	dfaMisses         int
	dfaFallbacks      int
}

// dfaTransitionKey identifies a cached transition by state and input rune.
type dfaTransitionKey struct {
	state int
	r     rune
}

// isDirectExpression reports whether a node can use the direct matcher.
func isDirectExpression(n *node, root bool) bool {
	if n.kind == nodeReference {
		return n.target != nil && isDirectExpression(n.target, root)
	}

	if root && n.kind == nodeRepeat {
		return isSingleRuneMatcher(n.children[0])
	}

	switch n.kind {
	case nodeLiteral:
		return true
	case nodeBuiltin:
		return !isBuiltinPattern(n.text)
	case nodeSequence:
		for _, child := range n.children {
			if !isDirectExpression(child, false) {
				return false
			}
		}

		return true
	default:
		return false
	}
}

// compileNFA builds an NFA for root, returning false when it is unsupported or too large.
func compileNFA(root *node) (*nfaProgram, bool) {
	builder := nfaBuilder{}

	fragment, ok := builder.compile(root)
	if !ok {
		return nil, false
	}

	for i := range builder.states {
		sort.Slice(builder.states[i].literalTransitions, func(a, b int) bool {
			return builder.states[i].literalTransitions[a].literal < builder.states[i].literalTransitions[b].literal
		})
	}

	return &nfaProgram{states: builder.states, start: fragment.start, accept: fragment.accept}, true
}

// newState appends a state unless the NFA state limit has been reached.
func (b *nfaBuilder) newState() (int, bool) {
	if len(b.states) >= maxNFAStates {
		return 0, false
	}

	id := len(b.states)
	b.states = append(b.states, nfaState{})

	return id, true
}

// addEpsilon adds a transition that consumes no input.
func (b *nfaBuilder) addEpsilon(from, to int) {
	b.states[from].epsilon = append(b.states[from].epsilon, to)
}

// compile converts an expression node into an NFA fragment.
func (b *nfaBuilder) compile(n *node) (nfaFragment, bool) {
	switch n.kind {
	case nodeLiteral:
		start, ok := b.newState()
		if !ok {
			return nfaFragment{}, false
		}

		current := start

		for _, r := range n.literalRunes {
			next, ok := b.newState()
			if !ok {
				return nfaFragment{}, false
			}

			b.states[current].literalTransitions = append(b.states[current].literalTransitions, nfaTransition{target: next, literal: r})
			current = next
		}

		return nfaFragment{start: start, accept: current}, true
	case nodeBuiltin:
		if isBuiltinPattern(n.text) {
			return nfaFragment{}, false
		}

		start, ok := b.newState()
		if !ok {
			return nfaFragment{}, false
		}

		accept, ok := b.newState()
		if !ok {
			return nfaFragment{}, false
		}

		b.states[start].builtinTransitions = append(b.states[start].builtinTransitions, nfaTransition{target: accept, builtin: n.text})

		return nfaFragment{start: start, accept: accept}, true
	case nodeReference:
		if n.target == nil {
			return nfaFragment{}, false
		}

		return b.compile(n.target)
	case nodeSequence:
		if len(n.children) == 0 {
			return nfaFragment{}, false
		}

		fragment, ok := b.compile(n.children[0])
		if !ok {
			return nfaFragment{}, false
		}

		for _, child := range n.children[1:] {
			next, ok := b.compile(child)
			if !ok {
				return nfaFragment{}, false
			}

			b.addEpsilon(fragment.accept, next.start)
			fragment.accept = next.accept
		}

		return fragment, true
	case nodeAlternative:
		if words, ok := collectLiteralWords(n); ok && len(words) > 1 {
			return b.compileLiteralTrie(words)
		}

		start, ok := b.newState()
		if !ok {
			return nfaFragment{}, false
		}

		accept, ok := b.newState()
		if !ok {
			return nfaFragment{}, false
		}

		literalWords := make([][]rune, 0)

		otherChildren := make([]*node, 0, len(n.children))
		for _, child := range n.children {
			words, ok := collectLiteralWords(child)
			if !ok || len(literalWords)+len(words) > maxLiteralAlternatives {
				otherChildren = append(otherChildren, child)
				continue
			}

			literalWords = append(literalWords, words...)
		}

		if len(literalWords) > 0 {
			trie, ok := b.compileLiteralTrie(literalWords)
			if !ok {
				return nfaFragment{}, false
			}

			b.addEpsilon(start, trie.start)
			b.addEpsilon(trie.accept, accept)
		}

		for _, child := range otherChildren {
			fragment, ok := b.compile(child)
			if !ok {
				return nfaFragment{}, false
			}

			b.addEpsilon(start, fragment.start)
			b.addEpsilon(fragment.accept, accept)
		}

		return nfaFragment{start: start, accept: accept}, true
	case nodeRepeat:
		start, ok := b.newState()
		if !ok {
			return nfaFragment{}, false
		}

		current := start

		for count := 0; count < n.min; count++ {
			fragment, ok := b.compile(n.children[0])
			if !ok {
				return nfaFragment{}, false
			}

			b.addEpsilon(current, fragment.start)
			current = fragment.accept
		}

		if n.max < 0 {
			accept, ok := b.newState()
			if !ok {
				return nfaFragment{}, false
			}

			fragment, ok := b.compile(n.children[0])
			if !ok {
				return nfaFragment{}, false
			}

			b.addEpsilon(current, accept)
			b.addEpsilon(current, fragment.start)
			b.addEpsilon(fragment.accept, current)

			return nfaFragment{start: start, accept: accept}, true
		}

		for count := n.min; count < n.max; count++ {
			next, ok := b.newState()
			if !ok {
				return nfaFragment{}, false
			}

			fragment, ok := b.compile(n.children[0])
			if !ok {
				return nfaFragment{}, false
			}

			b.addEpsilon(current, next)
			b.addEpsilon(current, fragment.start)
			b.addEpsilon(fragment.accept, next)
			current = next
		}

		return nfaFragment{start: start, accept: current}, true
	default:
		return nfaFragment{}, false
	}
}

// collectLiteralWords expands a literal-only expression into its possible words.
func collectLiteralWords(n *node) ([][]rune, bool) {
	switch n.kind {
	case nodeLiteral:
		return [][]rune{n.literalRunes}, true
	case nodeReference:
		if n.target == nil {
			return nil, false
		}

		return collectLiteralWords(n.target)
	case nodeAlternative:
		words := make([][]rune, 0, len(n.children))
		for _, child := range n.children {
			childWords, ok := collectLiteralWords(child)
			if !ok || len(words)+len(childWords) > maxLiteralAlternatives {
				return nil, false
			}

			words = append(words, childWords...)
		}

		return words, len(words) > 0
	case nodeSequence:
		words := [][]rune{{}}

		for _, child := range n.children {
			childWords, ok := collectLiteralWords(child)
			if !ok || len(words)*len(childWords) > maxLiteralAlternatives {
				return nil, false
			}

			combined := make([][]rune, 0, len(words)*len(childWords))
			for _, prefix := range words {
				for _, suffix := range childWords {
					word := make([]rune, 0, len(prefix)+len(suffix))
					word = append(word, prefix...)
					word = append(word, suffix...)
					combined = append(combined, word)
				}
			}

			words = combined
		}

		return words, len(words) > 0
	default:
		return nil, false
	}
}

// compileFixedLiteralRepeat recognizes repeats of equal-width literal choices.
func compileFixedLiteralRepeat(root *node) (*fixedLiteralRepeat, bool) {
	for root.kind == nodeReference && root.target != nil {
		root = root.target
	}

	if root.kind != nodeRepeat || isSingleRuneMatcher(root.children[0]) {
		return nil, false
	}

	words, ok := collectLiteralWords(root.children[0])
	if !ok || len(words) == 0 || len(words[0]) == 0 {
		return nil, false
	}

	width := len(words[0])
	for _, word := range words[1:] {
		if len(word) != width {
			return nil, false
		}
	}

	return &fixedLiteralRepeat{words: words, min: root.min, max: root.max}, true
}

// matches reports whether input consists of an allowed number of repeated words.
func (p *fixedLiteralRepeat) matches(input string) bool {
	count := 0
	for len(input) > 0 {
		if p.max >= 0 && count >= p.max {
			return false
		}

		matched := false

		for _, word := range p.words {
			remainder := input
			wordMatches := true

			for _, expected := range word {
				if len(remainder) == 0 {
					wordMatches = false
					break
				}

				actual, size := utf8.DecodeRuneInString(remainder)
				if actual != expected {
					wordMatches = false
					break
				}

				remainder = remainder[size:]
			}

			if wordMatches {
				input = remainder
				count++
				matched = true

				break
			}
		}

		if !matched {
			return false
		}
	}

	return count >= p.min
}

// literalTrieNode represents a node in a trie of literal alternatives.
type literalTrieNode struct {
	children map[rune]*literalTrieNode
	terminal bool
}

// compileLiteralTrie builds an NFA fragment from literal words using a trie.
func (b *nfaBuilder) compileLiteralTrie(words [][]rune) (nfaFragment, bool) {
	root := &literalTrieNode{}
	for _, word := range words {
		current := root
		for _, r := range word {
			if current.children == nil {
				current.children = make(map[rune]*literalTrieNode)
			}

			if current.children[r] == nil {
				current.children[r] = &literalTrieNode{}
			}

			current = current.children[r]
		}

		current.terminal = true
	}

	start, ok := b.newState()
	if !ok {
		return nfaFragment{}, false
	}

	accept, ok := b.newState()
	if !ok {
		return nfaFragment{}, false
	}

	if !b.compileTrieNode(root, start, accept) {
		return nfaFragment{}, false
	}

	return nfaFragment{start: start, accept: accept}, true
}

// compileTrieNode adds a trie node and its descendants to the NFA.
func (b *nfaBuilder) compileTrieNode(node *literalTrieNode, state, accept int) bool {
	if node.terminal {
		b.addEpsilon(state, accept)
	}

	runes := make([]rune, 0, len(node.children))
	for r := range node.children {
		runes = append(runes, r)
	}

	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })

	for _, r := range runes {
		next, ok := b.newState()
		if !ok {
			return false
		}

		b.states[state].literalTransitions = append(b.states[state].literalTransitions, nfaTransition{target: next, literal: r})
		if !b.compileTrieNode(node.children[r], next, accept) {
			return false
		}
	}

	return true
}

// literalTarget finds the transition for r and reports the number of comparisons.
func (s *nfaState) literalTarget(r rune) (int, bool, int) {
	transitions := s.literalTransitions
	if len(transitions) == 1 {
		if transitions[0].literal == r {
			return transitions[0].target, true, 1
		}

		return 0, false, 1
	}

	low, high, checks := 0, len(transitions), 0
	for low < high {
		checks++

		mid := low + (high-low)/2
		if transitions[mid].literal < r {
			low = mid + 1
		} else {
			high = mid
		}
	}

	index := low
	if index < len(transitions) {
		checks++
	}

	if index < len(transitions) && transitions[index].literal == r {
		return transitions[index].target, true, checks
	}

	return 0, false, checks
}

// matches reports whether input is accepted by the NFA program.
func (n *nfaProgram) matches(input string) bool {
	var workspace nfaWorkspace
	return workspace.match(n, input)
}

// match runs the NFA against input, using cached DFA transitions when available.
func (w *nfaWorkspace) match(program *nfaProgram, input string) bool {
	if w.stats != nil {
		*w.stats = nfaStats{}
	}

	w.prepareCache(program)

	stateCount := len(program.states)
	if cap(w.seen) < stateCount {
		w.seen = make([]uint32, stateCount)
	} else {
		w.seen = w.seen[:stateCount]
	}

	if cap(w.active) < stateCount {
		w.active = make([]int, 0, stateCount)
	} else {
		w.active = w.active[:0]
	}

	if cap(w.next) < stateCount {
		w.next = make([]int, 0, stateCount)
	} else {
		w.next = w.next[:0]
	}

	if cap(w.stack) < stateCount {
		w.stack = make([]int, 0, stateCount)
	} else {
		w.stack = w.stack[:0]
	}

	w.nextGeneration()
	w.active = w.active[:0]
	w.addClosure(program, program.start, &w.active)
	active := w.active
	currentState := w.startState

	cacheable := w.startCached
	if !cacheable {
		currentState, cacheable = w.cacheState(active)
		if cacheable {
			w.startState = currentState
			w.startCached = true
		}
	}

	for len(input) > 0 {
		r, size := utf8.DecodeRuneInString(input)

		key := dfaTransitionKey{state: currentState, r: r}
		if cacheable {
			if nextState, ok := w.transitions[key]; ok {
				if w.stats != nil {
					w.stats.dfaHits++
				}

				if nextState < 0 {
					return false
				}

				currentState = nextState
				active = w.stateSets[currentState]
				input = input[size:]

				continue
			}
		}

		if w.stats != nil {
			w.stats.dfaMisses++
		}

		w.nextGeneration()

		w.next = w.next[:0]
		for _, stateID := range active {
			if w.stats != nil {
				w.stats.activeStateVisits++
			}

			state := &program.states[stateID]
			if target, ok, checks := state.literalTarget(r); ok {
				w.addClosure(program, target, &w.next)

				if w.stats != nil {
					w.stats.transitionChecks += checks
				}
			} else if w.stats != nil {
				w.stats.transitionChecks += checks
			}

			for _, transition := range state.builtinTransitions {
				if w.stats != nil {
					w.stats.transitionChecks++
				}

				if transition.matches(r) {
					w.addClosure(program, transition.target, &w.next)
				}
			}
		}

		if len(w.next) == 0 {
			if cacheable {
				w.cacheTransition(key, -1)
			}

			return false
		}

		if cacheable {
			if nextState, ok := w.cacheState(w.next); ok && w.cacheTransition(key, nextState) {
				currentState = nextState
				active = w.stateSets[currentState]
				input = input[size:]

				continue
			}

			cacheable = false

			if w.stats != nil {
				w.stats.dfaFallbacks++
			}
		}

		w.active = append(w.active[:0], w.next...)
		active = w.active
		input = input[size:]
	}

	for _, stateID := range active {
		if stateID == program.accept {
			return true
		}
	}

	return false
}

// prepareCache clears the workspace caches when matching a different program.
func (w *nfaWorkspace) prepareCache(program *nfaProgram) {
	if w.program == program {
		return
	}

	w.program = program
	w.stateSets = w.stateSets[:0]
	w.stateIDs = make(map[string]int)
	w.transitions = make(map[dfaTransitionKey]int)
	w.stateEntries = 0
	w.cacheFull = false
	w.startState = 0
	w.startCached = false
}

// cacheState returns the cached DFA ID for states, adding it when capacity permits.
func (w *nfaWorkspace) cacheState(states []int) (int, bool) {
	if w.cacheFull {
		return 0, false
	}

	keyBytes := make([]byte, (len(w.seen)+7)/8)
	for _, state := range states {
		keyBytes[state/8] |= 1 << (state % 8)
	}

	key := string(keyBytes)
	if state, ok := w.stateIDs[key]; ok {
		return state, true
	}

	if len(w.stateSets) >= maxDFAStates || w.stateEntries+len(states) > maxDFAStateSetEntries {
		w.cacheFull = true
		return 0, false
	}

	state := len(w.stateSets)
	w.stateIDs[key] = state
	w.stateSets = append(w.stateSets, append([]int(nil), states...))
	w.stateEntries += len(states)

	return state, true
}

// cacheTransition stores a DFA transition unless the cache has reached its limit.
func (w *nfaWorkspace) cacheTransition(key dfaTransitionKey, target int) bool {
	if _, exists := w.transitions[key]; exists {
		return true
	}

	if w.cacheFull || len(w.transitions) >= maxDFATransitions {
		w.cacheFull = true
		return false
	}

	w.transitions[key] = target

	return true
}

// addClosure adds stateID and all epsilon-reachable states to active.
func (w *nfaWorkspace) addClosure(program *nfaProgram, stateID int, active *[]int) {
	w.stack = append(w.stack[:0], stateID)
	for len(w.stack) > 0 {
		last := len(w.stack) - 1
		current := w.stack[last]

		w.stack = w.stack[:last]
		if w.seen[current] == w.generation {
			continue
		}

		if w.stats != nil {
			w.stats.closureVisits++
		}

		w.seen[current] = w.generation
		*active = append(*active, current)
		w.stack = append(w.stack, program.states[current].epsilon...)
	}
}

// nextGeneration advances the visitation marker, clearing it if it wraps around.
func (w *nfaWorkspace) nextGeneration() {
	w.generation++
	if w.generation == 0 {
		clear(w.seen)
		w.generation = 1
	}
}

// matches reports whether r satisfies this transition's literal or built-in matcher.
func (t nfaTransition) matches(r rune) bool {
	if t.builtin != "" {
		return builtinMatches(t.builtin, r)
	}

	return t.literal == r
}
