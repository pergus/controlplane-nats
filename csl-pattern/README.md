# CSL Pattern Prototype

A small, dependency-free Go prototype for the proposed CSL string-pattern notation.

## Implemented syntax

- `type Name = expression`
- `pattern Name { expression }`
- Implicit concatenation: `A B`
- Alternatives: `A | B`
- Grouping: `(A B)`
- Prefix repetition: `repeat expression MIN to MAX`
- `unlimited` as the maximum repetition bound
- `optional expression`
- Quoted string literals, with `\"`, `\\`, `\n`, `\r`, and `\t` escapes
- Named type/pattern references
- Full-string matching (the entire input must match)
- `RegexToPattern`, which translates supported Go regular expressions into CSL expressions

The repetition operand is one atom. Use parentheses to repeat a compound expression:

```csl
repeat (letters | digits) 1 to 8
```

The `type` and `pattern` declarations currently compile to the same pattern expression representation. The distinction is retained in the source grammar for future CSL type-system work.

## Convert an email regular expression

The [email.regex](email.regex) example uses a common ASCII email-address shape. Convert it to a complete CSL definition with:

```sh
go run . -regex-file email.regex -name EmailAddress > generated-email.csl
```

Inspect the generated definition, then test the same address against it and the hand-written [email.csl](email.csl) pattern:

```sh
cat generated-email.csl
go run . -file generated-email.csl -pattern EmailAddress "john.doe@example.com"
go run . -file email.csl -pattern EmailAddress "john.doe@example.com"
```

Both match the ASCII example address and print `valid`. `-name` is optional and defaults to `RegexPattern`. CSL matching consumes the full input, so `^` and `$` are optional. `RegexToPattern` preserves Go regex character-class semantics: `[A-Za-z0-9]` maps to the ASCII-only `ascii_letters` and `ascii_digits` atoms. The hand-written example uses Unicode-aware `letters` and `digits`, so it also accepts addresses such as `josé@例子.com`; the ASCII regex-derived pattern does not.

The Go API is also available: `RegexToPattern` returns just the expression, while the CLI wraps it in a named definition.

## Built-in character atoms

- `digit`, `digits`
- `letter`, `letters`
- `lowercase_letter`, `lowercase_letters`
- `uppercase_letter`, `uppercase_letters`
- `ascii_letter`, `ascii_letters`
- `ascii_digit`, `ascii_digits`
- `whitespace`, `whitespace_character`
- `any_character`
- `any_character_except_newline`

Character categories use Go's Unicode character classification, except the explicitly ASCII-only category.

## Built-in pattern atoms

These atoms match strings rather than individual characters:

- `CamelCase`: starts with a lowercase letter, followed by letters or digits.
- `PascalCase`: starts with an uppercase letter, followed by letters or digits.
- `SnakeCase`: lowercase letter-started segments separated by single underscores; digits are allowed after each segment's first letter.
- `KebabCase`: lowercase letter-started segments separated by single hyphens; digits are allowed after each segment's first letter.
- `URL`: an absolute HTTP or HTTPS URL with a non-empty host and no whitespace.

For example, `pattern WebURL { URL }` matches `https://example.com/path`.

## Matching performance

Compilation chooses a matcher for each definition. Simple literals and character-atom sequences, top-level repetitions of single-rune matchers, pure literal alternatives, prefix-free literal choices in sequences, and fixed-width repetitions of literal alternatives use direct scans. Literal-only alternatives embedded in more complex expressions compile into shared-prefix tries with indexed rune transitions. Other regular combinations of literals, character atoms, sequences, alternatives, and repetitions compile to a Thompson NFA. NFA workspaces lazily cache transitions between active-state sets, with fixed state, transition, and memory caps; after a cap is reached, matching falls back to NFA simulation. Multi-character built-ins such as `URL` and `CamelCase`, or patterns that exceed the NFA state limit, use the memoized interpreter.

For repeated matches against a compiled program, create one `Matcher` with `program.NewMatcher()` and call its `Match` method. It reuses rune, NFA state-set, and lazy-cache buffers; once warmed, NFA benchmark patterns match with zero allocations. Create a separate matcher for each concurrent goroutine. `Program.Match` remains available for one-off matches.

Run the direct-CSL, NFA, reusable-CSL, and Go-regex benchmarks with `task benchmark`. Patterns are compiled before the timed loops; the first aligned table reports match time and allocations per operation. A second table reports cold NFA active-state visits, transition checks, closure visits, and DFA cache hits/misses for each NFA case. These work counters come from a separate single-match probe and do not add instrumentation overhead to the timed run. A separate schema-regex pipeline table reports `CSLConvert`, `CSLCompile`, `RegexCompile`, precompiled `CSLMatch`/`RegexMatch`, and `CSLEndToEnd`/`RegexEndToEnd`. In a server, conversion and compilation normally happen when registering a schema; match-only rows represent per-resource validation cost. The end-to-end rows show one-shot cost for comparison. The default sample duration is 100 ms per benchmark; set `BENCHTIME=1s` for longer measurements.

The benchmark corpus includes alternatives with disjoint or shared prefixes, nested alternatives and repetitions, repeated compound expressions, equal-length early and final failures, short/medium/long NFA inputs, and the schema specification's `username` and `domain_name` regex constraints.

Run the benchmark and generate both plots in one command:

```sh
./venv/bin/python plot.py
```

This writes `csl_vs_regex_speed_ratio.png` and `csl_benchmark_latency.png` to the current directory. To plot a benchmark table without rerunning the benchmark, save its output and pass it to the script:

```sh
task benchmark | tee benchmark.txt
./venv/bin/python plot.py --input benchmark.txt
```

## Run

```sh
go run . -file email.csl -list
go run . -file email.csl -pattern EmailAddress "john.doe@example.com"
```

Or build:

```sh
go build -o csl-pattern .
./csl-pattern -file email.csl -pattern EmailAddress "john.doe@example.com"
```

The program prints `valid` or `invalid`. It exits with status 0 for valid, 1 for invalid, and 2 for usage or compilation errors.

## Limitations

This is a pattern-language prototype, not a complete CSL implementation or a complete email validator.

- No Unicode normalization or IDNA/domain-name validation.
- No RFC-complete email validation.
- `RegexToPattern` supports literals, alternatives, concatenation, captures, repetition, bounded character classes, and absolute text anchors. Complete ASCII letter and digit ranges map to Unicode-aware built-ins. It returns an expression, not a complete named definition.
- Regex word-boundary assertions, line-specific anchors, lookarounds, backreferences, empty character classes, and character classes larger than 512 runes are not supported by `RegexToPattern`.
- The regex converter uses Go regular-expression syntax and targets CSL's full-string matching behavior.
- Recursive pattern definitions are rejected.
- No source maps or detailed match-failure explanations yet.
- No static proof that a pattern is satisfiable.
- `letters` and `digits` mean one Unicode character per occurrence; use `repeat letters 1 to unlimited` for a non-empty sequence.
- The grammar currently accepts only the documented built-ins and named definitions.

The matcher evaluates alternatives and repetitions over Unicode rune positions and requires a complete input match.
