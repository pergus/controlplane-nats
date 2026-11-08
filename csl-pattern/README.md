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

## Matcher selection and execution

When a CSL definition is compiled, the compiler chooses the most suitable matcher for each pattern.

- **Direct scans** are used for simple patterns, including literals, sequences of character atoms, top-level repetitions of single-rune matchers, pure literal alternatives, prefix-free literal choices within sequences, and fixed-width repetitions of literal alternatives.
- **Shared-prefix tries** are used when literal-only alternatives appear inside more complex expressions. These tries share common prefixes and use indexed rune transitions.
- **Thompson NFAs** handle other combinations of literals, character atoms, sequences, alternatives, and repetitions. NFA workspaces lazily cache transitions between active-state sets. The cache has fixed limits on states, transitions, and memory. When a limit is reached, matching continues through regular NFA simulation.
- **The memoized interpreter** handles multi-character built-ins such as `URL` and `CamelCase`, as well as patterns that exceed the NFA state limit.

### Reusing matchers

When matching multiple inputs against the same compiled program, create a reusable `Matcher` with `program.NewMatcher()` and call its `Match` method for each input.

A `Matcher` reuses its internal rune buffers, NFA state-set buffers, and lazy-cache storage. After warm-up, the NFA benchmark patterns run with zero allocations.

Create a separate `Matcher` for each concurrent goroutine. For one-off matches, you can continue to use `Program.Match`.

## Running the benchmarks

Run the benchmark suite with:

```bash
task benchmark
```

The suite compares four matching strategies:

- Direct CSL matching
- NFA matching
- Reusable CSL matching
- Go's regular expression engine

Patterns are compiled before the timed benchmark loops, so the main benchmark results measure matching performance rather than compilation time.

The output contains three sections.

### 1. Matching performance

The first table reports the execution time and memory allocations for each pattern and matcher. It includes:

- Number of benchmark iterations (`N`)
- Average matching time (`ns/op`)
- Memory allocated per operation (`Bytes/op`)
- Allocations per operation (`Allocs/op`)

### 2. NFA execution statistics

The second table reports the work performed by the NFA during a single match:

- **Active:** active-state visits
- **Checks:** transition checks
- **Closure:** closure visits
- **Hits:** DFA cache hits
- **Misses:** DFA cache misses

These counters are collected using a separate, instrumented single-match probe. They do not add instrumentation overhead to the timed benchmark results.

### 3. Schema regex pipeline

The final table measures the individual stages of converting and executing schema regular expressions:

- `CSLConvert`: converts a regular expression into a CSL definition.
- `CSLCompile`: compiles the resulting CSL definition.
- `RegexCompile`: compiles the original regular expression using Go's regex engine.
- `CSLMatch`: matches using an already-compiled CSL pattern.
- `RegexMatch`: matches using an already-compiled Go regex.
- `CSLEndToEnd`: measures the complete CSL conversion, compilation, and matching process.
- `RegexEndToEnd`: measures the complete Go regex compilation and matching process.

In a server application, conversion and compilation normally happen when a schema is registered, rather than for every resource being validated. Therefore, the match-only results represent the cost of validating individual resources, while the end-to-end results show the cost of processing a pattern from scratch.

The default benchmark duration is 100 ms per benchmark. To run longer measurements, use:

```bash
BENCHTIME=1s task benchmark
```

## Benchmark coverage

The benchmark corpus covers a range of pattern structures and input scenarios:

- Alternatives with disjoint prefixes and shared prefixes
- Nested alternatives and repetitions
- Repeated compound expressions
- Early and final failures using equal-length inputs
- Short, medium, and long NFA inputs
- The schema specification's `username` and `domain_name` regular expression constraints

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
