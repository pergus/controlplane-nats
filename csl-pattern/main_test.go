package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var allocationMatchResult bool

// emailPatterns is the test CSL definition for email-address matching.
const emailPatterns = `
type EmailName = repeat (
    letters | digits | "." | "_" | "%" | "+" | "-"
) 1 to unlimited
type SubDomain = repeat (letters | digits | "-") 1 to unlimited
type Domain = repeat ("." SubDomain) 1 to unlimited
pattern EmailAddress { EmailName "@" SubDomain Domain }
`

// TestEmailAddress checks valid and invalid examples against the email definition.
func TestEmailAddress(t *testing.T) {
	p, err := Compile(emailPatterns)
	if err != nil {
		t.Fatal(err)
	}

	valid := []string{
		"john.doe@example.com",
		"john+work@example.co.uk",
		"user_123@mail.example.org",
	}
	for _, value := range valid {
		ok, err := p.Match("EmailAddress", value)
		if err != nil || !ok {
			t.Errorf("expected %q to match, got ok=%v err=%v", value, ok, err)
		}
	}

	invalid := []string{
		"john.doe@localhost",
		"@example.com",
		"john@example",
		"john doe@example.com",
	}
	for _, value := range invalid {
		ok, err := p.Match("EmailAddress", value)
		if err != nil {
			t.Fatal(err)
		}

		if ok {
			t.Errorf("expected %q not to match", value)
		}
	}
}

// TestBuiltInNameAndURLPatterns checks the multi-character naming and URL atoms.
func TestBuiltInNameAndURLPatterns(t *testing.T) {
	p, err := Compile(`
pattern Camel { CamelCase }
pattern Pascal { PascalCase }
pattern Snake { SnakeCase }
pattern Kebab { KebabCase }
pattern WebURL { URL }
pattern Combined { CamelCase "_" SnakeCase }
`)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"Camel", "myVariable2", true},
		{"Camel", "MyVariable", false},
		{"Pascal", "MyVariable2", true},
		{"Pascal", "myVariable", false},
		{"Snake", "my_variable2", true},
		{"Snake", "my__variable", false},
		{"Kebab", "my-variable2", true},
		{"Kebab", "my--variable", false},
		{"WebURL", "https://example.com/path?q=1", true},
		{"WebURL", "ftp://example.com", false},
		{"WebURL", "https://example.com/path with space", false},
		{"Combined", "myVariable_value", true},
	}
	for _, test := range cases {
		got, err := p.Match(test.pattern, test.value)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.want {
			t.Errorf("Match(%s, %q) = %v, want %v", test.pattern, test.value, got, test.want)
		}
	}
}

// TestRepeatPrefixAndAlternation checks grouped alternatives with bounded repetition.
func TestRepeatPrefixAndAlternation(t *testing.T) {
	p, err := Compile(`
pattern Code { ("vm-" | "ct-") repeat (lowercase_letter | digit) 2 to 4 }
`)
	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"vm-a1", "ct-1234"} {
		ok, err := p.Match("Code", value)
		if err != nil || !ok {
			t.Errorf("expected %q to match, got ok=%v err=%v", value, ok, err)
		}
	}

	for _, value := range []string{"net-a1", "vm-a", "ct-12345"} {
		ok, err := p.Match("Code", value)
		if err != nil {
			t.Fatal(err)
		}

		if ok {
			t.Errorf("expected %q not to match", value)
		}
	}
}

func TestSingleRuneRepeatMatches(t *testing.T) {
	program, err := Compile(`
pattern Lower { repeat lowercase_letter 2 to 4 }
pattern UnicodeRunes { repeat "é" 2 to 3 }
pattern CharacterClass { repeat (letter | digit) 1 to 3 }
pattern Optional { optional "x" }
`)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"Lower", "ab", true},
		{"Lower", "abcde", false},
		{"Lower", "A", false},
		{"UnicodeRunes", "éé", true},
		{"UnicodeRunes", "ééé", true},
		{"UnicodeRunes", "éééé", false},
		{"CharacterClass", "é٣a", true},
		{"CharacterClass", "x!", false},
		{"Optional", "", true},
		{"Optional", "x", true},
		{"Optional", "xx", false},
	}
	for _, test := range cases {
		got, err := program.Match(test.pattern, test.value)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.want {
			t.Errorf("Match(%s, %q) = %v, want %v", test.pattern, test.value, got, test.want)
		}
	}
}

func TestGeneralRepeatAndSequenceMatches(t *testing.T) {
	program, err := Compile(`
pattern Ambiguous { repeat ("a" | "aa") 1 to 3 }
pattern Nullable { repeat optional "x" 0 to unlimited }
pattern Nested { repeat (repeat ("a" | "aa") 0 to unlimited) 0 to unlimited }
pattern Sequence { ("a" | "ab") ("b" | "bc") }
`)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"Ambiguous", "", false},
		{"Ambiguous", "a", true},
		{"Ambiguous", "aaa", true},
		{"Ambiguous", "aaaaaa", true},
		{"Ambiguous", "aaaaaaa", false},
		{"Nullable", "", true},
		{"Nullable", "xxxx", true},
		{"Nullable", "y", false},
		{"Nested", "", true},
		{"Nested", "aaaa", true},
		{"Nested", "b", false},
		{"Sequence", "ab", true},
		{"Sequence", "abc", true},
		{"Sequence", "abbc", true},
		{"Sequence", "ac", false},
	}
	for _, test := range cases {
		got, err := program.Match(test.pattern, test.value)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.want {
			t.Errorf("Match(%s, %q) = %v, want %v", test.pattern, test.value, got, test.want)
		}
	}
}

func TestSimplePatternMatchAllocations(t *testing.T) {
	program, err := Compile(`
pattern Literal { "hello" }
pattern Sequence { letter digit }
pattern Repeated { repeat ("a" | "b") 1 to 5 }
`)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		pattern string
		value   string
	}{
		{"Literal", "hello"},
		{"Sequence", "é٣"},
		{"Repeated", "abba"},
	}
	for _, test := range cases {
		allocations := testing.AllocsPerRun(100, func() {
			matched, err := program.Match(test.pattern, test.value)
			allocationMatchResult = matched

			if err != nil {
				t.Fatal(err)
			}
		})
		if allocations != 0 {
			t.Errorf("Match(%s, %q) allocated %v times per call, want 0", test.pattern, test.value, allocations)
		}
	}
}

func TestReusableMatcherAcrossInputs(t *testing.T) {
	program, err := Compile(`pattern Code { repeat (letter | digit) 1 to 6 "-" "ok" }`)
	if err != nil {
		t.Fatal(err)
	}

	matcher := program.NewMatcher()

	cases := []struct {
		value string
		want  bool
	}{
		{"abc-oko", false},
		{"abc-ok", true},
		{"é٣-ok", true},
		{"1234567-ok", false},
		{"x-ok", true},
	}
	for _, test := range cases {
		got, err := matcher.Match("Code", test.value)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.want {
			t.Errorf("Match(%q) = %v, want %v", test.value, got, test.want)
		}
	}

	allocations := testing.AllocsPerRun(100, func() {
		matched, err := matcher.Match("Code", "abc-ok")
		allocationMatchResult = matched

		if err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Errorf("warm reusable Match allocated %v times per call, want 0", allocations)
	}
}

func TestCompileSelectsMatcherStrategy(t *testing.T) {
	program, err := Compile(`
pattern Literal { "hello" }
pattern CharacterRepeat { repeat letter 1 to unlimited }
pattern Choice { ("cat" | "dog" | lowercase_letter) repeat digit 1 to 3 }
pattern LiteralChoice { "alpha" | "alpine" }
pattern PrefixChoice { ("cat" | "car") digit }
pattern AmbiguousChoice { ("a" | "ab") digit }
pattern CompoundRepeat { repeat ("ab" | "cd") 1 to 4 }
pattern WebURL { URL }
pattern TooLarge { repeat ("xy" | digit) 0 to 4096 }
`)
	if err != nil {
		t.Fatal(err)
	}

	strategies := map[string]matchStrategy{
		"Literal":         strategyDirect,
		"CharacterRepeat": strategyDirect,
		"Choice":          strategyNFA,
		"LiteralChoice":   strategyDirect,
		"PrefixChoice":    strategyDirect,
		"AmbiguousChoice": strategyDirect,
		"CompoundRepeat":  strategyDirect,
		"WebURL":          strategyInterpreter,
		"TooLarge":        strategyInterpreter,
	}
	for name, want := range strategies {
		if got := program.definitions[name].strategy; got != want {
			t.Errorf("strategy for %s = %v, want %v", name, got, want)
		}
	}

	regex, err := regexp.Compile(`\A(?:(?:cat|dog|\p{Ll})\p{Nd}{1,3})\z`)
	if err != nil {
		t.Fatal(err)
	}

	matcher := program.NewMatcher()

	for _, value := range []string{"cat7", "dog123", "dog٣", "a7", "cat", "dog1234", "bird7"} {
		want := regex.MatchString(value)
		for _, match := range []func(string, string) (bool, error){program.Match, matcher.Match} {
			got, err := match("Choice", value)
			if err != nil {
				t.Fatal(err)
			}

			if got != want {
				t.Errorf("Match(%q) = %v, regex oracle = %v", value, got, want)
			}
		}
	}

	prefixChoiceOracle, err := regexp.Compile(`\A(?:cat|car)\p{Nd}\z`)
	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"cat7", "car٣", "cab7", "cat"} {
		want := prefixChoiceOracle.MatchString(value)
		for _, match := range []func(string, string) (bool, error){program.Match, matcher.Match} {
			got, err := match("PrefixChoice", value)
			if err != nil {
				t.Fatal(err)
			}

			if got != want {
				t.Errorf("PrefixChoice(%q) = %v, regex oracle = %v", value, got, want)
			}
		}
	}

	for _, test := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"ab", true},
		{"cdab", true},
		{"abcdcd", true},
		{"ababababab", false},
		{"ef", false},
	} {
		got, err := matcher.Match("CompoundRepeat", test.value)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.want {
			t.Errorf("CompoundRepeat(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestNFACacheAndFallback(t *testing.T) {
	program, err := Compile(`pattern Choice { ("a" | "ab" | lowercase_letter) digit }`)
	if err != nil {
		t.Fatal(err)
	}

	machine := program.definitions["Choice"].nfa
	stats := &nfaStats{}

	workspace := nfaWorkspace{stats: stats}
	for index, value := range []string{"ab7", "a2", "ax1", "ab7"} {
		got := workspace.match(machine, value)

		want := value == "ab7" || value == "a2"
		if got != want {
			t.Errorf("NFA match %q = %v, want %v", value, got, want)
		}

		if index == 0 && (stats.closureVisits == 0 || stats.activeStateVisits == 0 || stats.transitionChecks == 0 || stats.dfaMisses == 0) {
			t.Fatal("cold NFA work counters were not populated")
		}
	}

	if len(workspace.transitions) == 0 {
		t.Fatal("NFA did not cache any transitions")
	}

	if stats.dfaHits == 0 {
		t.Fatal("repeated NFA input did not hit the transition cache")
	}

	workspace.cacheFull = true
	if !workspace.match(machine, "ab3") {
		t.Fatal("NFA fallback rejected a valid input")
	}

	if stats.dfaFallbacks == 0 {
		t.Fatal("NFA fallback counter was not incremented")
	}
}

// TestRegexToPattern compares generated patterns with anchored Go regular expressions.
func TestRegexToPattern(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		values     []string
	}{
		{
			name:       "classes and bounded repetition",
			expression: `^[a-c]+[0-2]{1,2}$`,
			values:     []string{"a0", "abc12", "d0", "a123", ""},
		},
		{
			name:       "grouped alternation and digits",
			expression: `(?:cat|dog)\d+`,
			values:     []string{"cat1", "dog22", "cat", "xcat1", "dogA"},
		},
		{
			name:       "alternation and optional",
			expression: `(?:cat|dog)?`,
			values:     []string{"", "cat", "dog", "cats"},
		},
		{
			name:       "case folding",
			expression: `(?i:go)`,
			values:     []string{"go", "GO", "Go", "gO", "g"},
		},
		{
			name:       "dot and newline",
			expression: `(?s:.)`,
			values:     []string{"\n", "x", ""},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			generated, err := RegexToPattern(test.expression)
			if err != nil {
				t.Fatal(err)
			}

			program, err := Compile("pattern Generated { " + generated + " }")
			if err != nil {
				t.Fatalf("generated invalid CSL expression %q: %v", generated, err)
			}

			oracle, err := regexp.Compile(`\A(?:` + test.expression + `)\z`)
			if err != nil {
				t.Fatal(err)
			}

			for _, value := range test.values {
				got, err := program.Match("Generated", value)
				if err != nil {
					t.Fatal(err)
				}

				want := oracle.MatchString(value)
				if got != want {
					t.Errorf("value %q: generated match=%v, regex match=%v", value, got, want)
				}
			}
		})
	}
}

// TestRegexToPatternUsesCharacterBuiltins checks complete ASCII ranges map to CSL atoms.
func TestRegexToPatternUsesCharacterBuiltins(t *testing.T) {
	expression, err := RegexToPattern(`[A-Za-z0-9_]`)
	if err != nil {
		t.Fatal(err)
	}

	if expression != "(\n    ascii_letters\n    | ascii_digits\n    | \"_\"\n)" {
		t.Fatalf("generated expression = %q, want built-in character atoms", expression)
	}

	program, err := Compile("pattern WordCharacter { " + expression + " }")
	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"A", "7", "_"} {
		matched, err := program.Match("WordCharacter", value)
		if err != nil {
			t.Fatal(err)
		}

		if !matched {
			t.Errorf("expected built-in expression to match %q", value)
		}
	}

	for _, value := range []string{"é", "٣", "-", "ab"} {
		matched, err := program.Match("WordCharacter", value)
		if err != nil {
			t.Fatal(err)
		}

		if matched {
			t.Errorf("expected built-in expression not to match %q", value)
		}
	}
}

// TestRegexToPatternRejectsUnsupportedSyntax checks that nonrepresentable regexes fail explicitly.
func TestRegexToPatternRejectsUnsupportedSyntax(t *testing.T) {
	for _, expression := range []string{`\bword\b`, `a^b`, `\pL`} {
		if _, err := RegexToPattern(expression); err == nil {
			t.Errorf("expected %q to be rejected", expression)
		}
	}
}

// TestConvertRegexFile checks conversion from a regex file to a usable CSL definition.
func TestConvertRegexFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "code.regex")
	if err := os.WriteFile(path, []byte("go-[0-2]{2}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	source, err := convertRegexFile(path, "GoCode")
	if err != nil {
		t.Fatal(err)
	}

	program, err := Compile(source)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		value string
		want  bool
	}{{"go-12", true}, {"go-13", false}, {"xgo-12", false}} {
		got, err := program.Match("GoCode", test.value)
		if err != nil {
			t.Fatal(err)
		}

		if got != test.want {
			t.Errorf("Match(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

// TestEmailRegexConversion preserves regex ASCII classes and hand-written Unicode classes.
func TestEmailRegexConversion(t *testing.T) {
	generated, err := convertRegexFile("email.regex", "EmailAddress")
	if err != nil {
		t.Fatal(err)
	}

	generatedProgram, err := Compile(generated)
	if err != nil {
		t.Fatal(err)
	}

	emailSource, err := os.ReadFile("email.csl")
	if err != nil {
		t.Fatal(err)
	}

	handwrittenProgram, err := Compile(string(emailSource))
	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"john.doe@example.com", "john+work@example.co.uk", "john@example"} {
		generatedMatch, err := generatedProgram.Match("EmailAddress", value)
		if err != nil {
			t.Fatal(err)
		}

		handwrittenMatch, err := handwrittenProgram.Match("EmailAddress", value)
		if err != nil {
			t.Fatal(err)
		}

		if generatedMatch != handwrittenMatch {
			t.Errorf("email definitions disagree for %q: regex=%v, CSL=%v", value, generatedMatch, handwrittenMatch)
		}
	}

	generatedUnicodeMatch, err := generatedProgram.Match("EmailAddress", "josé@例子.com")
	if err != nil {
		t.Fatal(err)
	}

	if generatedUnicodeMatch {
		t.Error("generated ASCII regex pattern unexpectedly matched Unicode email characters")
	}

	handwrittenUnicodeMatch, err := handwrittenProgram.Match("EmailAddress", "josé@例子.com")
	if err != nil {
		t.Fatal(err)
	}

	if !handwrittenUnicodeMatch {
		t.Error("hand-written Unicode-aware CSL pattern rejected Unicode email characters")
	}
}
