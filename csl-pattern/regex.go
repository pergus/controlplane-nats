package main

import (
	"fmt"
	"regexp/syntax"
	"sort"
	"strings"
	"unicode"
)

// maxRegexClassRunes is the maximum number of runes that can be translated from
// a regexp character class.
// maxRegexClassRunes bounds character-class expansion during regex conversion.
const maxRegexClassRunes = 512

// RegexToPattern translates a Go regular expression into a CSL expression.
// The expression is intended for this package's full-string matcher.
func RegexToPattern(expression string) (string, error) {
	parsed, err := syntax.Parse(expression, syntax.Perl)
	if err != nil {
		return "", fmt.Errorf("parse regular expression: %w", err)
	}

	generated, err := renderRegex(parsed, true, true)
	if err != nil {
		return "", err
	}

	return prettyPatternExpression(generated)
}

// prettyPatternExpression parses and formats a generated CSL expression.
func prettyPatternExpression(expression string) (string, error) {
	p, err := newParser(expression)
	if err != nil {
		return "", err
	}

	root, err := p.parseExpression()
	if err != nil {
		return "", err
	}

	if p.current().kind != tokEOF {
		return "", fmt.Errorf("unexpected token %q in generated expression", p.current().text)
	}

	return formatPatternNode(root, 0), nil
}

// formatPatternNode prints a CSL node with indentation for nested expressions.
func formatPatternNode(n *node, depth int) string {
	indent := strings.Repeat("    ", depth)

	switch n.kind {
	case nodeLiteral:
		return quotePatternRunes([]rune(n.text))
	case nodeBuiltin, nodeReference:
		return n.text
	case nodeSequence:
		parts := make([]string, len(n.children))
		for i, child := range n.children {
			parts[i] = formatPatternNode(child, depth)
		}

		return strings.Join(parts, "\n"+indent)
	case nodeAlternative:
		return "(\n" + formatPatternAlternativeContents(n, depth+1) + "\n" + indent + ")"
	case nodeRepeat:
		maximum := "unlimited"
		if n.max >= 0 {
			maximum = fmt.Sprint(n.max)
		}

		child := n.children[0]

		childExpression := strings.Repeat("    ", depth+1) + formatPatternNode(child, depth+1)
		if child.kind == nodeAlternative {
			childExpression = formatPatternAlternativeContents(child, depth+1)
		}

		return "repeat (\n" + childExpression +
			"\n" + indent + fmt.Sprintf(") %d to %s", n.min, maximum)
	default:
		return ""
	}
}

// formatPatternAlternativeContents prints alternatives without surrounding parentheses.
func formatPatternAlternativeContents(n *node, depth int) string {
	parts := make([]string, len(n.children))
	for i, child := range n.children {
		prefix := ""
		if i > 0 {
			prefix = "| "
		}

		parts[i] = strings.Repeat("    ", depth) + prefix + formatPatternNode(child, depth)
	}

	return strings.Join(parts, "\n")
}

// renderRegex converts one regexp syntax node while tracking text-boundary anchors.
func renderRegex(re *syntax.Regexp, atStart, atEnd bool) (string, error) {
	switch re.Op {
	case syntax.OpNoMatch:
		return "", fmt.Errorf("regular expression language cannot be represented by CSL")
	case syntax.OpEmptyMatch:
		return `""`, nil
	case syntax.OpBeginText, syntax.OpEndText:
		if re.Op == syntax.OpBeginText && !atStart {
			return "", fmt.Errorf("begin-text anchor is only supported at the start")
		}

		if re.Op == syntax.OpEndText && !atEnd {
			return "", fmt.Errorf("end-text anchor is only supported at the end")
		}

		return "", nil
	case syntax.OpLiteral:
		return renderRegexLiteral(re)
	case syntax.OpCharClass:
		return renderRegexClass(re)
	case syntax.OpAnyCharNotNL:
		return "any_character_except_newline", nil
	case syntax.OpAnyChar:
		return "any_character", nil
	case syntax.OpCapture:
		return renderRegex(re.Sub[0], atStart, atEnd)
	case syntax.OpConcat:
		parts := make([]string, 0, len(re.Sub))

		first, last := 0, len(re.Sub)-1
		for first < len(re.Sub) && re.Sub[first].Op == syntax.OpEmptyMatch {
			first++
		}

		for last >= 0 && re.Sub[last].Op == syntax.OpEmptyMatch {
			last--
		}

		for i, child := range re.Sub {
			part, err := renderRegex(child, atStart && i == first, atEnd && i == last)
			if err != nil {
				return "", err
			}

			if part != "" {
				parts = append(parts, part)
			}
		}

		if len(parts) == 0 {
			return `""`, nil
		}

		return strings.Join(parts, " "), nil
	case syntax.OpAlternate:
		parts := make([]string, len(re.Sub))
		for i, child := range re.Sub {
			part, err := renderRegex(child, atStart, atEnd)
			if err != nil {
				return "", err
			}

			parts[i] = part
		}

		return "(" + strings.Join(parts, " | ") + ")", nil
	case syntax.OpStar:
		return renderRegexRepeat(re.Sub[0], 0, -1)
	case syntax.OpPlus:
		return renderRegexRepeat(re.Sub[0], 1, -1)
	case syntax.OpQuest:
		return renderRegexRepeat(re.Sub[0], 0, 1)
	case syntax.OpRepeat:
		return renderRegexRepeat(re.Sub[0], re.Min, re.Max)
	case syntax.OpBeginLine, syntax.OpEndLine, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return "", fmt.Errorf("regular expression assertion %s is not supported", re.Op)
	default:
		return "", fmt.Errorf("regular expression operation %s is not supported", re.Op)
	}
}

// renderRegexLiteral quotes literal runes and expands case-folded runes as alternatives.
func renderRegexLiteral(re *syntax.Regexp) (string, error) {
	if re.Flags&syntax.FoldCase == 0 {
		return quotePatternRunes(re.Rune), nil
	}

	parts := make([]string, len(re.Rune))
	for i, r := range re.Rune {
		alternatives, err := renderFoldedRune(r)
		if err != nil {
			return "", err
		}

		parts[i] = alternatives
	}

	return strings.Join(parts, " "), nil
}

// renderRegexClass expands a regexp character class into quoted alternatives.
func renderRegexClass(re *syntax.Regexp) (string, error) {
	runes := make(map[rune]bool)

	for i := 0; i+1 < len(re.Rune); i += 2 {
		low, high := re.Rune[i], re.Rune[i+1]
		for r := low; r <= high; r++ {
			if !runes[r] {
				runes[r] = true
				if len(runes) > maxRegexClassRunes {
					return "", fmt.Errorf("character class exceeds the %d-rune translation limit", maxRegexClassRunes)
				}
			}

			if r == high {
				break
			}
		}
	}

	if re.Flags&syntax.FoldCase != 0 {
		for r := range runes {
			for folded := nextFoldedRune(r); folded != r; folded = nextFoldedRune(folded) {
				runes[folded] = true
				if len(runes) > maxRegexClassRunes {
					return "", fmt.Errorf("case-folded character class exceeds the %d-rune translation limit", maxRegexClassRunes)
				}
			}
		}
	}

	if len(runes) == 0 {
		return "", fmt.Errorf("empty character classes cannot be represented by CSL")
	}

	parts := make([]string, 0, len(runes))
	if containsRuneRange(runes, 'A', 'Z') && containsRuneRange(runes, 'a', 'z') {
		parts = append(parts, "ascii_letters")

		removeRuneRange(runes, 'A', 'Z')
		removeRuneRange(runes, 'a', 'z')
	}

	if containsRuneRange(runes, '0', '9') {
		parts = append(parts, "ascii_digits")

		removeRuneRange(runes, '0', '9')
	}

	ordered := make([]rune, 0, len(runes))
	for r := range runes {
		ordered = append(ordered, r)
	}

	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	for _, r := range ordered {
		parts = append(parts, quotePatternRunes([]rune{r}))
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("empty character classes cannot be represented by CSL")
	}

	if len(parts) == 1 {
		return parts[0], nil
	}

	return "(" + strings.Join(parts, " | ") + ")", nil
}

// containsRuneRange reports whether every rune in the inclusive range is present.
func containsRuneRange(runes map[rune]bool, low, high rune) bool {
	for r := low; r <= high; r++ {
		if !runes[r] {
			return false
		}
	}

	return true
}

// removeRuneRange deletes every rune in the inclusive range from the set.
func removeRuneRange(runes map[rune]bool, low, high rune) {
	for r := low; r <= high; r++ {
		delete(runes, r)
	}
}

// renderFoldedRune returns a quoted rune or its simple Unicode case-fold alternatives.
func renderFoldedRune(r rune) (string, error) {
	runes := map[rune]bool{r: true}
	for folded := nextFoldedRune(r); folded != r; folded = nextFoldedRune(folded) {
		runes[folded] = true
	}

	ordered := make([]rune, 0, len(runes))
	for folded := range runes {
		ordered = append(ordered, folded)
	}

	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	parts := make([]string, len(ordered))
	for i, folded := range ordered {
		parts[i] = quotePatternRunes([]rune{folded})
	}

	if len(parts) == 1 {
		return parts[0], nil
	}

	return "(" + strings.Join(parts, " | ") + ")", nil
}

// nextFoldedRune returns the next rune in Go's Unicode simple-fold cycle.
func nextFoldedRune(r rune) rune {
	return unicode.SimpleFold(r)
}

// renderRegexRepeat wraps a translated expression in CSL repetition syntax.
func renderRegexRepeat(child *syntax.Regexp, min, max int) (string, error) {
	expression, err := renderRegex(child, false, false)
	if err != nil {
		return "", err
	}

	maximum := "unlimited"
	if max >= 0 {
		maximum = fmt.Sprint(max)
	}

	return fmt.Sprintf("repeat (%s) %d to %s", expression, min, maximum), nil
}

// quotePatternRunes returns a DSL string literal with supported escapes applied.
func quotePatternRunes(runes []rune) string {
	var out strings.Builder
	out.WriteByte('"')

	for _, r := range runes {
		switch r {
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		case '\\', '"':
			out.WriteByte('\\')
			out.WriteRune(r)
		default:
			out.WriteRune(r)
		}
	}

	out.WriteByte('"')

	return out.String()
}
