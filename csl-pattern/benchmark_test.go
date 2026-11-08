package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var benchmarkMatchResult bool
var benchmarkStringResult string
var benchmarkProgramResult *Program
var benchmarkRegexResult *regexp.Regexp

var patternBenchmarks = []struct {
	name  string
	csl   string
	regex string
	value string
}{
	{"literal_hello", `"hello"`, `hello`, "hello"},
	{"literal_version", `"v1.2.3"`, `v1\.2\.3`, "v1.2.3"},
	{"literal_email", `"name@example.com"`, `name@example\.com`, "name@example.com"},
	{"literal_hyphen", `"task-42"`, `task-42`, "task-42"},
	{"literal_underscore", `"user_id"`, `user_id`, "user_id"},
	{"single_letter", `letter`, `\pL`, "g"},
	{"single_digit", `digit`, `\p{Nd}`, "7"},
	{"letters", `repeat letters 1 to unlimited`, `\pL+`, "pattern"},
	{"digits", `repeat digits 1 to unlimited`, `\p{Nd}+`, "2048"},
	{"lowercase_ab", `repeat ("a" | "b") 1 to unlimited`, `[ab]+`, "abba"},
	{"uppercase_CD", `repeat ("C" | "D") 1 to unlimited`, `[CD]+`, "DCCD"},
	{"ascii_letters", `repeat ascii_letters 1 to unlimited`, `[A-Za-z]+`, "GoLang"},
	{"whitespace", `whitespace_character`, `[\t-\r\x{85}\x{a0}\p{Z}]`, " "},
	{"any_character", `any_character`, `(?s:.)`, "x"},
	{"character_except_newline", `any_character_except_newline`, `.`, "x"},
	{"one_to_three_digits", `repeat digit 1 to 3`, `\p{Nd}{1,3}`, "123"},
	{"exactly_four_digits", `repeat digit 4 to 4`, `\p{Nd}{4}`, "2048"},
	{"one_or_more_letters", `repeat letter 1 to unlimited`, `\pL+`, "example"},
	{"bounded_alphanumeric", `repeat (letter | digit) 1 to 8`, `[\pL\p{Nd}]{1,8}`, "abc123"},
	{"optional_x", `repeat "x" 0 to 1`, `x?`, "x"},
	{"two_or_more_x", `repeat "x" 2 to unlimited`, `x{2,}`, "xxxx"},
	{"letter_then_digit", `letter digit`, `\pL\p{Nd}`, "a7"},
	{"cat_or_dog", `"cat" | "dog"`, `(?:cat|dog)`, "dog"},
	{"prefix_alternative", `("vm-" | "ct-") repeat digits 1 to unlimited`, `(?:vm-|ct-)\p{Nd}+`, "ct-123"},
	{"item_number", `"item-" repeat digits 1 to unlimited`, `item-\p{Nd}+`, "item-42"},
	{"identifier", `ascii_letter repeat (ascii_letter | digit | "_") 0 to unlimited`, `[A-Za-z][A-Za-z\p{Nd}_]*`, "user_7"},
	{"dotted_version", `repeat digits 1 to unlimited "." repeat digits 1 to unlimited "." repeat digits 1 to unlimited`, `\p{Nd}+\.\p{Nd}+\.\p{Nd}+`, "1.22.3"},
	{"date", `repeat digit 4 to 4 "-" repeat digit 2 to 2 "-" repeat digit 2 to 2`, `\p{Nd}{4}-\p{Nd}{2}-\p{Nd}{2}`, "2026-11-08"},
	{"hex_color", `repeat (digit | "a" | "b" | "c" | "d" | "e" | "f") 6 to 6`, `[\p{Nd}a-f]{6}`, "a1b2c3"},
	{"phone_number", `repeat digits 1 to unlimited "-" repeat digits 1 to unlimited "-" repeat digits 1 to unlimited`, `\p{Nd}+-\p{Nd}+-\p{Nd}+`, "555-010-2048"},
	{"alternatives_disjoint", `"cat" | "dog" | "bird" | "fish" | "tree" | "cloud"`, `(?:cat|dog|bird|fish|tree|cloud)`, "cloud"},
	{"alternatives_shared_prefix", `"common-prefix-0123456789-abcdefgh-alpha" | "common-prefix-0123456789-abcdefgh-bravo" | "common-prefix-0123456789-abcdefgh-charlie" | "common-prefix-0123456789-abcdefgh-delta"`, `(?:common-prefix-0123456789-abcdefgh-alpha|common-prefix-0123456789-abcdefgh-bravo|common-prefix-0123456789-abcdefgh-charlie|common-prefix-0123456789-abcdefgh-delta)`, "common-prefix-0123456789-abcdefgh-charlie"},
	{"alternatives_shared_prefix_nfa", `("common-prefix-0123456789-abcdefgh-alpha" | "common-prefix-0123456789-abcdefgh-alpha-extended" | lowercase_letter) digit`, `(?:common-prefix-0123456789-abcdefgh-alpha|common-prefix-0123456789-abcdefgh-alpha-extended|\p{Ll})\p{Nd}`, "common-prefix-0123456789-abcdefgh-alpha-extended7"},
	{"nested_alternatives", `(("red" | "green") | ("blue" | "yellow"))`, `(?:(?:red|green)|(?:blue|yellow))`, "yellow"},
	{"nested_alternatives_nfa", `(("red" | "green") | ("blue" | "bluebird") | lowercase_letter) digit`, `(?:(?:red|green)|(?:blue|bluebird)|\p{Ll})\p{Nd}`, "bluebird7"},
	{"nested_repetitions", `repeat (repeat "a" 0 to 2 "b") 1 to 3`, `(?:a{0,2}b){1,3}`, "aabbaab"},
	{"repeated_compound", `repeat ("ab" | "cd") 1 to 6`, `(?:ab|cd){1,6}`, "abcdababcd"},
	{"repeated_compound_nfa", `repeat ("ab" | "cd" | letter) 1 to 6`, `(?:ab|cd|\pL){1,6}`, "abcdab"},
	{"failure_early", `repeat ascii_letter 1 to unlimited "END"`, `[A-Za-z]+END`, "!" + strings.Repeat("a", 255) + "END"},
	{"failure_final", `repeat ascii_letter 1 to unlimited "END"`, `[A-Za-z]+END`, strings.Repeat("a", 256) + "ENX"},
	{"success_short", `repeat ("ab" | "cd" | digit) 1 to 128`, `(?:ab|cd|\p{Nd}){1,128}`, "ab"},
	{"success_medium", `repeat ("ab" | "cd" | digit) 1 to 128`, `(?:ab|cd|\p{Nd}){1,128}`, strings.Repeat("ab", 32)},
	{"success_long", `repeat ("ab" | "cd" | digit) 1 to 128`, `(?:ab|cd|\p{Nd}){1,128}`, strings.Repeat("ab", 128)},
	{"schema_username", "", `^[a-z][a-z0-9_]{2,31}$`, "controlplane_1"},
	{"schema_domain_name", "", `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, "api-server-7"},
}

func BenchmarkPatternMatching(b *testing.B) {
	for _, test := range patternBenchmarks {
		csl := test.csl
		var err error
		if csl == "" {
			csl, err = RegexToPattern(test.regex)
			if err != nil {
				b.Fatalf("convert schema regex %q: %v", test.name, err)
			}
		}

		program, err := Compile("pattern Benchmark { " + csl + " }")
		if err != nil {
			b.Fatalf("compile CSL pattern %q: %v", test.name, err)
		}

		compiledRegex, err := regexp.Compile(`\A(?:` + test.regex + `)\z`)
		if err != nil {
			b.Fatalf("compile regex for %q: %v", test.name, err)
		}

		for _, value := range []string{test.value, "", "é", "٣", "ª", "Ⅻ", " ", "\t", "\u00a0", "\n", "x", "abc", "A7"} {
			cslMatched, err := program.Match("Benchmark", value)
			if err != nil {
				b.Fatalf("match CSL pattern %q: %v", test.name, err)
			}

			if regexMatched := compiledRegex.MatchString(value); cslMatched != regexMatched {
				b.Fatalf("pattern %q disagrees for %q: CSL=%v regex=%v", test.name, value, cslMatched, regexMatched)
			}
		}

		want := test.name != "failure_early" && test.name != "failure_final"
		if matched, err := program.Match("Benchmark", test.value); err != nil || matched != want {
			b.Fatalf("benchmark input %q matched pattern %q as %v, want %v (err=%v)", test.value, test.name, matched, want, err)
		}
		reusableMatcher := program.NewMatcher()
		if matched, err := reusableMatcher.Match("Benchmark", test.value); err != nil || matched != want {
			b.Fatalf("reusable matcher matched %q as %v, want %v (err=%v)", test.value, matched, want, err)
		}

		engine := "Interpreter"
		var coldNFAStats nfaStats
		switch program.definitions["Benchmark"].strategy {
		case strategyDirect:
			engine = "Direct"
		case strategyNFA:
			engine = "NFA"
			diagnostic := nfaWorkspace{stats: &coldNFAStats}
			diagnostic.match(program.definitions["Benchmark"].nfa, test.value)
		}

		b.Run(fmt.Sprintf("%s/%s", engine, test.name), func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				benchmarkMatchResult, _ = program.Match("Benchmark", test.value)
			}
			if engine == "NFA" {
				b.ReportMetric(float64(coldNFAStats.closureVisits), "cold-closure/op")
				b.ReportMetric(float64(coldNFAStats.activeStateVisits), "cold-active/op")
				b.ReportMetric(float64(coldNFAStats.transitionChecks), "cold-checks/op")
				b.ReportMetric(float64(coldNFAStats.dfaMisses), "cold-misses/op")
				b.ReportMetric(float64(coldNFAStats.dfaHits), "cold-hits/op")
			}
		})

		b.Run(fmt.Sprintf("Regex/%s", test.name), func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				benchmarkMatchResult = compiledRegex.MatchString(test.value)
			}
		})

		b.Run(fmt.Sprintf("Reuse/%s", test.name), func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				benchmarkMatchResult, _ = reusableMatcher.Match("Benchmark", test.value)
			}
		})
	}
}

var schemaRegexBenchmarks = []struct {
	name  string
	regex string
	value string
}{
	{"schema_username", `^[a-z][a-z0-9_]{2,31}$`, "controlplane_1"},
	{"schema_domain_name", `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, "api-server-7"},
}

func BenchmarkSchemaRegexPipeline(b *testing.B) {
	for _, test := range schemaRegexBenchmarks {
		expression, err := RegexToPattern(test.regex)
		if err != nil {
			b.Fatalf("convert %s regex: %v", test.name, err)
		}
		source := "pattern SchemaPattern { " + expression + " }"
		program, err := Compile(source)
		if err != nil {
			b.Fatalf("compile converted %s pattern: %v", test.name, err)
		}
		compiledRegex, err := regexp.Compile(`\A(?:` + test.regex + `)\z`)
		if err != nil {
			b.Fatalf("compile %s regex: %v", test.name, err)
		}
		if matched, err := program.Match("SchemaPattern", test.value); err != nil || !matched || !compiledRegex.MatchString(test.value) {
			b.Fatalf("schema pattern preflight failed for %s: matched=%v err=%v", test.name, matched, err)
		}

		b.Run(test.name+"/CSLConvert", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkStringResult, _ = RegexToPattern(test.regex)
			}
		})

		b.Run(test.name+"/CSLCompile", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkProgramResult, _ = Compile(source)
			}
		})

		b.Run(test.name+"/RegexCompile", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkRegexResult, _ = regexp.Compile(`\A(?:` + test.regex + `)\z`)
			}
		})

		b.Run(test.name+"/CSLMatch", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkMatchResult, _ = program.Match("SchemaPattern", test.value)
			}
		})

		b.Run(test.name+"/RegexMatch", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkMatchResult = compiledRegex.MatchString(test.value)
			}
		})

		b.Run(test.name+"/CSLEndToEnd", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				converted, conversionErr := RegexToPattern(test.regex)
				if conversionErr != nil {
					b.Fatal(conversionErr)
				}
				compiled, compileErr := Compile("pattern SchemaPattern { " + converted + " }")
				if compileErr != nil {
					b.Fatal(compileErr)
				}
				benchmarkMatchResult, _ = compiled.Match("SchemaPattern", test.value)
			}
		})

		b.Run(test.name+"/RegexEndToEnd", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				compiled, compileErr := regexp.Compile(`\A(?:` + test.regex + `)\z`)
				if compileErr != nil {
					b.Fatal(compileErr)
				}
				benchmarkMatchResult = compiled.MatchString(test.value)
			}
		})
	}
}
