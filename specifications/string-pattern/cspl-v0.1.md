# Controlplane String Pattern Language (CSPL)
## Specification — Draft v0.1

**Status:** Draft \
**Purpose:** Declarative string-pattern definitions for Controlplane \
**Implementation:** Prototype in `csl-pattern`

---

## 1. Introduction

Controlplane String Pattern Language (CSPL) is a declarative language for defining named patterns that match complete strings. A pattern can use literal text, character categories, named definitions, concatenation, alternatives, and repetition.

CSPL does not execute user-supplied code. A CSPL implementation compiles definitions and evaluates them against input strings. This draft specifies the syntax and behavior. It does not specify a resource schema, a regular-expression language, or an API for storing patterns.

The words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** express requirements within this draft.

## 2. Source files and definitions

A CSPL source consists of zero or more named pattern definitions:

```cspl
pattern Name { expression }
```

Each pattern names an expression that callers can match. A pattern can reference another named pattern.

Definition names MUST be unique in one source. A reference MAY name a definition that appears later in the source. A reference to an unknown name is an error. Recursive and mutually recursive definitions are not supported and MUST be rejected.

Definitions are case-sensitive. For example, `Email` and `email` are different names.

## 3. Lexical rules

CSPL ignores Unicode whitespace between tokens. Whitespace does not change the meaning of an expression except where it separates tokens. A semicolon (`;`) outside a string literal starts a comment. The comment continues to the end of the line; the line ending may be LF, CRLF, or CR. The semicolon and comment text are ignored. A semicolon inside a string literal is part of that literal.

An identifier starts with a Unicode letter or underscore. The remaining characters, if any, MUST be Unicode letters, Unicode digits, or underscores. Identifiers are case-sensitive.

A repetition bound is a non-negative ASCII decimal integer. The word `unlimited` is used only as the maximum bound for `repeat`.

A string literal is enclosed in double quotes. It matches the enclosed sequence of Unicode code points exactly. The empty string is written as `""`. The following escapes are supported:

| Escape | Meaning |
|---|---|
| `\\` | Backslash |
| `\"` | Double quote |
| `\n` | Line feed |
| `\r` | Carriage return |
| `\t` | Tab |

An escape not listed in this table is invalid. CSPL does not perform Unicode normalization or case folding on string literals.

## 4. Expressions

An expression is one or more sequences separated by `|`. A sequence is one or more adjacent terms. Adjacent terms are concatenated. Concatenation binds more tightly than alternation. Parentheses group an expression.

```cspl
pattern Scheme { "http" | "https" }
pattern HostLabel { lowercase_letter repeat (lowercase_letter | digit | "-") 0 to 61 }
pattern WebAddress { Scheme "://" "example.com" }
```

The first pattern matches either literal. `WebAddress` matches either scheme followed by the literal host shown in the pattern.

### 4.1 Repetition

The `repeat` form repeats one atom between a minimum and maximum number of times:

```cspl
repeat atom MIN to MAX
```

`MIN` and a numeric `MAX` MUST be non-negative integers. `MAX` MUST be greater than or equal to `MIN`. The maximum may instead be `unlimited`.

The repeated operand is one atom. Use parentheses to repeat a sequence or alternative:

```cspl
pattern ShortWord { repeat (letters | digits) 1 to 8 }
```

Repetition includes every count allowed by its bounds. Matching is not defined as greedy or non-greedy; a definition succeeds if any valid path consumes the complete input.

### 4.2 Optional terms

`optional atom` is equivalent to repeating the atom zero or one time:

```cspl
pattern OptionalSuffix { "item" optional ("-" digits) }
```

The optional operand is one atom. Parentheses group a compound operand.

## 5. Formal grammar

The following grammar describes the concrete syntax. `identifier`, `integer`, and `string` are lexical tokens as defined in Section 3. The built-in names listed in Section 6 are identifiers with predefined meanings.

```ebnf
source       = { definition } ;
definition   = "pattern", identifier, "{", expression, "}" ;
expression   = sequence, { "|", sequence } ;
sequence     = term, { term } ;
term         = "repeat", atom, integer, "to", (integer | "unlimited")
             | "optional", atom
             | atom ;
atom         = string
             | identifier
             | "(", expression, ")" ;
```

An identifier in an expression resolves to a built-in atom or a named pattern. The words `pattern`, `repeat`, `optional`, `to`, and `unlimited` have syntax-specific meaning in their grammar positions.

## 6. Built-in atoms

Built-ins are case-sensitive. Except for the string-matching built-ins in Section 6.2, each built-in matches exactly one Unicode code point.

### 6.1 Character built-ins

| Built-in names | Matches |
|---|---|
| `digit`, `digits` | A Unicode digit, as classified by Go's `unicode.IsDigit` |
| `letter`, `letters` | A Unicode letter, as classified by `unicode.IsLetter` |
| `lowercase_letter`, `lowercase_letters` | A Unicode lowercase letter, as classified by `unicode.IsLower` |
| `uppercase_letter`, `uppercase_letters` | A Unicode uppercase letter, as classified by `unicode.IsUpper` |
| `ascii_digit`, `ascii_digits` | One ASCII digit from `0` through `9` |
| `ascii_letter`, `ascii_letters` | One ASCII letter from `A` through `Z` or `a` through `z` |
| `whitespace`, `whitespace_character` | A Unicode whitespace code point, as classified by Go's `unicode.IsSpace` |
| `any_character` | Any Unicode code point, including line feed |
| `any_character_except_newline` | Any Unicode code point except line feed (`U+000A`) |

The names ending in `s` are aliases for one-character matchers. They do not match a sequence. Use `repeat` to match multiple characters, for example `repeat letters 1 to unlimited`.

### 6.2 String-matching built-ins

These built-ins match one or more code points. They can also match a prefix when used inside a larger expression; the complete definition must still match the full input.

| Built-in | Definition |
|---|---|
| `CamelCase` | Begins with a Unicode lowercase letter. Remaining characters, if any, are Unicode letters or digits. |
| `PascalCase` | Begins with a Unicode uppercase letter. Remaining characters, if any, are Unicode letters or digits. |
| `SnakeCase` | One or more segments separated by single underscores. Each segment starts with a Unicode lowercase letter and continues with Unicode lowercase letters or digits. |
| `KebabCase` | One or more segments separated by single hyphens. Each segment starts with a Unicode lowercase letter and continues with Unicode lowercase letters or digits. |
| `URL` | An absolute HTTP or HTTPS URL with a non-empty host name and no whitespace. Scheme matching is case-insensitive. |

The `URL` built-in uses Go URL parsing in the prototype. CSPL v0.1 does not define additional URL normalization or validation rules.

## 7. Matching semantics

A named definition matches an input only when the expression can consume the complete input. A match of only a prefix is not sufficient. Therefore, patterns do not need start or end anchors.

String literals compare exact code-point sequences. Character built-ins use the Unicode categories or ASCII ranges defined in Section 6. Alternatives succeed if any branch matches. Sequences succeed when each term matches in order. Repetition succeeds when a permitted number of repetitions matches.

A matcher MUST give the same result regardless of its internal implementation strategy. Implementations MAY use direct scans, automata, interpreters, or other equivalent methods.

## 8. Go regular-expression converter

The Go prototype provides `RegexToPattern` and a command-line option that converts a supported Go regular expression to a CSPL expression. This converter is a prototype utility, not part of the CSPL grammar. It returns an expression, not a complete named definition.

The converter supports literals, concatenation, alternatives, captures, repetition, supported character classes, and absolute text-boundary anchors. Captures do not create CSPL definitions. Anchors at the absolute beginning or end can be omitted because CSPL matching consumes the complete input.

The converter MUST report an error for unsupported constructs. The prototype does not support word-boundary assertions, line-specific anchors, lookarounds, backreferences, named capture groups, empty character classes, or character classes that exceed its 512-code-point expansion limit. Its input syntax is Go regular-expression syntax, not CSPL syntax.

## 9. Errors and limits

A compiler MUST reject malformed syntax, duplicate definition names, invalid repetition bounds, unsupported string escapes, unknown references, and recursive definitions. The exact error text and source-position format are implementation-defined.

CSPL v0.1 does not provide:

- Character-class syntax in CSPL source.
- Negation, intersection, or subtraction of patterns.
- Recursive definitions.
- Captures or extraction of matched substrings.
- Match explanations, source maps, or satisfiability proofs.
- Type declarations; reusable definitions are declared with `pattern`.

The regular-expression converter has the additional limits in Section 8. These converter limits do not restrict expressions written directly in CSPL.

## 10. Versioning

This document defines draft version 0.1. A future version may add syntax or semantics. A future version MUST document any change that affects whether existing source parses or what strings an existing pattern matches.
