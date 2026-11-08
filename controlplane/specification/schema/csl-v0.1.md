# Controlplane Schema Language (CSL)
## Formal Language Specification
**Version 0.1 — Draft**

---

## 1. Introduction

Controlplane Schema Language (CSL) is a declarative language for defining resource kinds, their attributes, data types, constraints, defaults, and validation rules.

CSL is designed for a dynamic control plane in which resource kinds and schema versions can be registered at runtime, without database schema migrations or service restarts.

The language is intended to be:

- **Declarative:** schemas describe data and its permitted values, not application logic.
- **Strongly typed:** every field has an explicit type.
- **Compact:** common structures have concise syntax.
- **Extensible:** users can define custom types.
- **Deterministic:** validation does not depend on external services or arbitrary code.
- **Versioned:** registered schema versions are immutable.
- **Patch-friendly:** partial updates modify only specified attributes, followed by validation of the resulting resource.

CSL does not prescribe a storage engine, database, message broker, or API implementation.

## 2. Lexical conventions

### 2.1 Comments

A semicolon begins a comment extending to the end of the line.

```text
; This is a comment.
cpu: int16 range 1 to 128 ; CPU allocation
```

### 2.2 Identifiers

Identifiers begin with a letter or underscore and may contain letters, digits, and underscores.

Examples:

```text
VirtualMachine
machine_model
number_of_disks
```

Identifiers are case-sensitive.

### 2.3 Strings

Strings use double quotes.

```text
name: string = "production"
```

Supported escapes:

| Escape | Meaning |
|---|---|
| `\"` | Double quote |
| `\\` | Backslash |
| `\n` | Newline |
| `\r` | Carriage return |
| `\t` | Tab |
| `\uXXXX` | Unicode code point |

### 2.4 Numeric literals

Integer and decimal literals are supported.

```text
cpu: int16 = 4
ratio: decimal = 1.25
```

Numeric literals may include units when the expected type supports them.

```text
memory: bytes = 8GiB
timeout: duration = "30s"
```

Supported byte units:

- Decimal: `B`, `KB`, `MB`, `GB`, `TB`, `PB`
- Binary: `KiB`, `MiB`, `GiB`, `TiB`, `PiB`

Byte values are represented internally as non-negative integer byte counts.

Duration values use duration strings with explicit units, such as `"30s"`, `"5m"`, or `"2h"`.

### 2.5 Whitespace

Whitespace separates tokens. Commas are optional between values in `oneof(...)`, but are not generally required or used as field terminators.

Braces define object and conditional scopes.

---

## 3. Built-in types

CSL provides primitive, numeric, temporal, networking, identifier, encoding, and specialized types.

### 3.1 Primitive types

| Type | Description |
|---|---|
| `string` | UTF-8 string |
| `bool` | Boolean |
| `boolean` | Alias for `bool` |
| `any` | Any supported CSL value |
| `json` | Arbitrary valid JSON value |

`any` accepts any supported CSL value, while `json` specifically represents JSON-compatible data.

### 3.2 Integer types

| Type | Description |
|---|---|
| `int` | Alias for `int64` |
| `int8` | Signed 8-bit integer |
| `int16` | Signed 16-bit integer |
| `int32` | Signed 32-bit integer |
| `int64` | Signed 64-bit integer |
| `uint` | Alias for `uint64` |
| `uint8` | Unsigned 8-bit integer |
| `uint16` | Unsigned 16-bit integer |
| `uint32` | Unsigned 32-bit integer |
| `uint64` | Unsigned 64-bit integer |

Integer types must reject fractional values and values outside their representable ranges.

### 3.3 Floating-point and decimal types

| Type | Description |
|---|---|
| `float32` | IEEE 754 32-bit floating point |
| `float64` | IEEE 754 64-bit floating point |
| `float` | Alias for `float64` |
| `decimal` | Arbitrary-precision decimal |
| `number` | General numeric value |

`decimal` is recommended for financial and precision-sensitive values. `number` is a convenience type and should not be used where exact numeric representation is required.

NaN and infinity are not valid values unless a future specification explicitly permits them.

### 3.4 Binary and size types

| Type | Description |
|---|---|
| `bytes` | Non-negative byte quantity |
| `binary` | Binary data |
| `base64` | Base64-encoded data |
| `hex` | Hexadecimal-encoded data |

`bytes` represents a numeric size, whereas `binary` represents actual binary content.

### 3.5 Date and time types

| Type | Description |
|---|---|
| `date` | Calendar date |
| `time` | Time of day |
| `timestamp` | Date and time with timezone |
| `datetime` | Alias for `timestamp` |
| `duration` | Time interval |
| `timezone` | Timezone identifier |

Canonical representations:

- `date`: ISO 8601 date, such as `2026-11-07`.
- `time`: ISO 8601 time.
- `timestamp`: RFC 3339 timestamp with timezone.
- `duration`: duration string with explicit units.
- `timezone`: IANA timezone identifier, such as `Europe/Stockholm`.

### 3.6 Identifier types

| Type | Description |
|---|---|
| `uuid` | UUID identifier |
| `uuid4` | UUID version 4 |
| `uuid7` | UUID version 7 |
| `ulid` | ULID identifier |

All identifier types are represented as strings in resource documents.

### 3.7 Networking types

| Type | Description |
|---|---|
| `ip` | IPv4 or IPv6 address |
| `ipv4` | IPv4 address |
| `ipv6` | IPv6 address |
| `cidr` | IPv4 or IPv6 network prefix |
| `ipv4_cidr` | IPv4 network prefix |
| `ipv6_cidr` | IPv6 network prefix |
| `mac` | MAC address |
| `hostname` | Hostname |
| `domain` | Domain name |
| `fqdn` | Fully qualified domain name |
| `dns` | DNS name |
| `url` | Absolute URL |
| `uri` | URI |
| `email` | Email address |
| `port` | TCP/UDP port number |
| `protocol` | Network protocol identifier |

Additional semantics:

- `ip` accepts either IPv4 or IPv6.
- `ipv4` and `ipv6` enforce their respective address formats.
- `cidr` accepts a network address with a prefix length.
- `mac` accepts a valid MAC address.
- `hostname` follows hostname label rules.
- `domain` represents a valid domain name.
- `fqdn` requires a fully qualified domain name.
- `dns` accepts a valid DNS name.
- `url` requires an absolute URL with a valid scheme.
- `uri` accepts a syntactically valid URI.
- `email` validates email address syntax, not mailbox existence.
- `port` is an integer from 0 to 65535.
- `protocol` accepts registered protocol identifiers and explicitly defined numeric protocol values.

DNS validation must not require an external DNS lookup.

### 3.8 Text and format types

| Type | Description |
|---|---|
| `alpha` | Alphabetic characters |
| `alphanumeric` | Alphabetic and numeric characters |
| `numeric` | Numeric characters represented as text |
| `hexadecimal` | Hexadecimal text |
| `path` | Generic path |
| `filepath` | File path |
| `directory` | Directory path |
| `crontab` | Cron expression |

Path types perform syntactic validation only. They must not access the filesystem or verify that a path exists.

`crontab` validates expression syntax; it does not execute schedules.

### 3.9 Cryptographic and certificate types

| Type | Description |
|---|---|
| `certificate` | X.509 certificate |
| `csr` | Certificate signing request |
| `pem` | PEM-encoded data |
| `public_key` | Public key |
| `private_key` | Private key |

These types validate encoding and, where applicable, structural validity. They do not imply that a certificate is trusted, a key is authorized, or a signature is valid.

A private key field should be treated as sensitive by the runtime and must not be exposed through ordinary resource reads, logs, or event payloads without an explicit security policy.

---

## 4. Compound types

### 4.1 Lists

Lists are declared by appending square brackets to a type.

```text
networks: string[]
```

This defines a list of strings with no cardinality restriction.

Cardinality can be specified inside the brackets.

```text
networks: string[1:8]
```

This defines a list containing at least one and at most eight strings.

Supported forms:

| Syntax | Meaning |
|---|---|
| `string[]` | Zero or more elements |
| `string[8]` | Zero to eight elements |
| `string[1:8]` | One to eight elements |
| `string[3:10]` | Three to ten elements |
| `string[5:5]` | Exactly five elements |
| `string[1:]` | At least one element |
| `string[:]` | Zero or more elements |

A single integer specifies the maximum number of elements. An omitted lower bound defaults to zero.

An omitted upper bound means unlimited, subject to runtime resource limits.

### 4.2 Nested lists

Lists can contain other lists.

```text
matrix: int16[][]
```

A list of up to ten lists, each containing at least one integer:

```text
matrix: int16[1:][1:10]
```

Cardinality applies to the immediately preceding type expression. Therefore, the rightmost suffix constrains the outermost list.

For readability, deeply nested lists should generally be represented using named custom types.

### 4.3 Maps

Maps use the following syntax:

```text
labels: map[string]string
```

The syntax is:

```text
map[key_type]value_type
```

Initially, map keys must be strings. Map values can be any supported CSL type.

Examples:

```text
labels: map[string]string
resources: map[string]ResourceLimits
groups: map[string]string[]
configuration: map[string]map[string]string
```

Map values may also have list cardinality constraints:

```text
groups: map[string]string[1:8]
```

This defines a map whose values are lists containing between one and eight strings.

Map keys must be unique. Duplicate keys in a resource document are invalid.

### 4.4 Objects

Objects contain named fields.

```text
ResourceLimits {
    cpu: int16
    memory: bytes
}
```

Objects may be nested:

```text
spec {
    cpu: int16
    memory: bytes

    backup {
        enabled: bool
        schedule: crontab
    }
}
```

Object fields follow the same declaration and validation rules as resource fields.

---

## 5. Custom types

Custom types allow users to define reusable types with additional restrictions.

### 5.1 Refined types

A refined type adds constraints to an existing type.

```text
type domain_name string where matches("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
```

Another example:

```text
type positive_integer int64 range 1+
```

A refined type inherits the validation rules of its underlying type and applies its additional constraints.

### 5.2 Enumerated types

An enumeration defines a finite set of permitted values.

```text
type machine_model string oneof(q35 i440fx virt pc)
```

Equivalent syntax with commas is also accepted:

```text
type machine_model string oneof(q35, i440fx, virt, pc)
```

Enumeration values may be identifiers, strings, numbers, or booleans.

```text
type database_engine string oneof(postgres mysql sqlite)
type replica_count int8 oneof(1 2 3 5)
type enabled_state bool oneof(true false)
```

Unquoted identifiers inside `oneof(...)` are interpreted as literal string values, not variable references.

### 5.3 Named object types

Reusable object types can be declared independently.

```text
type ResourceLimits {
    cpu: int16 range 1 to 128
    memory: bytes range 512MiB+
}
```

They can then be referenced by other types or resources.

```text
limits: ResourceLimits
```

### 5.4 Type restrictions

Custom types:

- Must have a valid underlying type.
- Must not redefine built-in types.
- Must not contain circular type references.
- Must be deterministic.
- Must not execute arbitrary code.
- Must be resolved when the schema is registered.

Custom types are scoped to their schema namespace unless the runtime explicitly supports shared type namespaces.

---

## 6. Resource definitions

### 6.1 Canonical declaration

A resource kind is declared using `resouce`, followed by its name and schema version.

```text
resource VirtualMachine version 1 {
    metadata {
        name: hostname
    }

    spec {
        cpu: int16
        memory: bytes
    }
}
```

A resource definition contains two primary sections:

- `metadata`: identity and descriptive information.
- `spec`: desired resource configuration.

### 6.2 System-managed fields

The control plane owns system-managed attributes.

These attributes are not declared by users in the schema:

| Attribute | Description |
|---|---|
| `kind` | Resource kind |
| `uid` | Immutable resource identifier |
| `generation` | Desired-state generation |
| `resource_version` | Version of the stored resource |
| `created_at` | Creation timestamp |
| `updated_at` | Last update timestamp |
| `deleted_at` | Deletion timestamp, when applicable |

The control plane is responsible for assigning and updating these fields.

`generation` increments when the desired specification changes.

`resource_version` changes whenever the stored resource changes, including metadata or status changes.

System-managed fields cannot be overwritten through ordinary user resource operations.

### 6.3 Metadata

Metadata contains resource identity and descriptive attributes.

```text
metadata {
    name: hostname
    namespace: string optional
    labels: map[string]string optional
    annotations: map[string]string optional
}
```

The runtime may reserve specific metadata fields, such as `name` and `namespace`, and enforce uniqueness or naming policies independently of the schema.

### 6.4 Specification

The `spec` section describes the desired state.

```text
spec {
    cpu: int16 range 1 to 128
    memory: bytes range 512MiB+
    image: url
}
```

The control plane stores the desired specification. Independent controllers may observe changes and reconcile external systems toward that desired state.

---

## 7. Field declarations

### 7.1 General syntax

A field declaration has the following structure:

```text
field_name: TYPE [constraints] [optional] [nullable] [= default]
```

Examples:

```text
cpu: int16 range 1 to 128
memory: bytes range 512MiB+
hostname: hostname
description: string optional
enabled: bool = true
```

### 7.2 Required fields

Fields are required by default.

```text
image: string
```

The field must be present when creating a resource and must remain present after an update.

### 7.3 Optional fields

The `optional` modifier allows a field to be omitted.

```text
description: string optional
```

An omitted optional field is not automatically assigned a value.

### 7.4 Nullable fields

The `nullable` modifier allows an explicitly supplied null value.

```text
description: string nullable
```

Optional and nullable are independent:

```text
description: string optional nullable
```

This field may be omitted or explicitly set to null.

### 7.5 Default values

Defaults are specified using `=`.

```text
enabled: bool = true
model: machine_model = q35
replicas: int16 = 1
```

Defaults are applied during resource creation when the field is omitted.

Defaults are not reapplied during updates. Removing an optional field does not automatically restore its default.

Default values must be valid instances of their declared types.

A default value is evaluated deterministically and must not depend on external services, current time, or arbitrary code.

---

## 8. Constraints

Constraints restrict the values a field may contain.

### 8.1 Numeric ranges

```text
cpu: int16 range 1 to 128
memory: bytes range 512MiB+
temperature: decimal range -40 to 125
```

Supported forms:

```text
range 1 to 128
range 1+
range 1 to *
range * to 128
```

`*` represents an unbounded endpoint.

Ranges are inclusive.

### 8.2 String length

```text
hostname: string length 1 to 63
description: string length 0 to 1024
```

Length is measured in Unicode code points, not UTF-8 bytes.

### 8.3 Pattern matching

```text
username: string matches("^[a-z][a-z0-9_]{2,31}$")
```

Patterns use a defined regular-expression dialect. The runtime must use the same dialect consistently across implementations.

### 8.4 Enumerations

A field can define its permitted values inline.

```text
model: string oneof(q35 i440fx virt pc)
```

Or use a named type:

```text
type machine_model string oneof(q35 i440fx virt pc)

model: machine_model
```

### 8.5 Collection cardinality

Collection cardinality is part of the type declaration.

```text
networks: string[1:8]
ports: port[1:16]
```

The separate `min_items`, `max_items`, and `unique` modifiers are not part of the canonical v0.2 syntax.

### 8.6 Uniqueness

The `unique` modifier may be used on lists when duplicate elements are not allowed.

```text
networks: string[1:8] unique
ports: port[1:16] unique
```

Uniqueness is evaluated using the canonical equality semantics of the element type.

---

## 9. Expressions

CSL supports a restricted expression language for validation and conditional declarations.

Expressions must be deterministic and side-effect-free.

### 9.1 Comparison operators

| Operator | Meaning |
|---|---|
| `==` | Equal |
| `!=` | Not equal |
| `<` | Less than |
| `<=` | Less than or equal |
| `>` | Greater than |
| `>=` | Greater than or equal |

### 9.2 Logical operators

```text
and
or
not
```

Examples:

```text
if spec.enabled == true and spec.replicas > 1 {
    ...
}
```

### 9.3 Membership

The `in` operator checks whether a value belongs to a set.

```text
if spec.model in (q35 i440fx) {
    ...
}
```

### 9.4 Arithmetic

The expression language supports:

```text
+
-
*
/
%
```

Arithmetic is restricted to compatible numeric types.

Division by zero is invalid. Overflow and precision behavior must be deterministic and defined by the operand types.

### 9.5 Field references

Fields are referenced using dot notation.

```text
spec.cpu
spec.memory
metadata.name
```

References are resolved against the resource being validated.

A reference to an unknown field is a schema registration error.

### 9.6 Functions

The initial specification permits a small set of built-in pure functions, including:

```text
matches(value, pattern)
```

Additional functions may be standardized in later versions.

User-defined functions and arbitrary code execution are not supported.

---

## 10. Conditional validation

Conditional declarations allow fields to become required or have different validation rules depending on other resource values.

### 10.1 Syntax

```text
if expression {
    ...
} else {
    ...
}
```

The `else` branch is optional.

### 10.2 Example

```text
resouce VirtualMachine version 1 {
    metadata {
        name: hostname
    }

    spec {
        monitored: bool = true

        if spec.monitored == true {
            monitoring {
                endpoint: url
                interval: duration = "30s"
                timeout: duration = "5s"
            }
        } else {
            monitoring {
                enabled: bool = false
            }
        }
    }
}
```

### 10.3 Semantics

- Only the active branch determines which conditional fields are required.
- Fields in inactive branches are not required.
- If an inactive-branch field is supplied, it must still conform to its declared type and constraints.
- Conditional declarations do not automatically create, delete, or modify fields.
- Conditional validation is evaluated against the complete resulting resource.
- Conditions must be deterministic and must not depend on external state.

When multiple branches declare the same field, their definitions must be compatible. The schema compiler must reject conflicting definitions.

A field declared only in one branch is conditionally required when that branch is active.

---

## 11. Resource validation

### 11.1 Creation

When creating a resource, the runtime must:

1. Resolve the resource kind and schema version.
2. Reject unknown fields.
3. Apply defaults to omitted fields.
4. Validate primitive and compound types.
5. Validate field constraints.
6. Evaluate conditional declarations.
7. Validate required fields.
8. Assign system-managed attributes.
9. Persist the resource and its creation event according to the control plane's transaction model.

### 11.2 Updates

Updates are partial by default.

For example, given:

```text
spec {
    cpu: 4
    memory: 8589934592
    image: "ubuntu-24.04"
}
```

A patch containing:

```text
spec {
    cpu: 8
}
```

changes only `cpu`. The existing `memory` and `image` values remain unchanged.

The runtime must:

1. Load the current resource.
2. Apply the supplied patch to the existing resource.
3. Validate the complete resulting resource.
4. Reject the update if any constraint is violated.
5. Persist the update atomically.
6. Increment `resource_version`.
7. Increment `generation` when the desired specification changes.

The runtime must not interpret an omitted field as deletion.

### 11.3 Removing fields

Field removal must be explicit.

The update API must provide an unambiguous removal operation, such as a dedicated `REMOVE` instruction or a structured patch deletion operation.

Removing a required field is invalid.

Removing an optional field is permitted.

Removing a field does not reapply its creation default.

### 11.4 Unknown fields

Unknown fields are rejected by default.

This prevents misspelled or unsupported fields from being silently accepted.

A future extension may support explicitly declared extension fields.

---

## 12. Schema versioning

Every resource kind has an explicit schema version.

```text
resource VirtualMachine version 1 {
    ...
}
```

A new schema version is registered as a separate immutable definition.

```text
resouce VirtualMachine version 2 {
    ...
}
```

Rules:

- Registered schema versions are immutable.
- Existing resources retain their recorded schema version until explicitly migrated.
- Schema registration must reject invalid definitions.
- Schema updates must not silently change existing resource interpretation.
- Migrations must be explicit and auditable.
- The control plane must be able to identify the schema version used to validate a resource.

A schema registry should support retrieving a kind's available versions and identifying its active version.

---

## 13. Formal grammar

The following EBNF describes the core CSL v0.2 syntax. Implementations may extend it only through explicitly versioned language extensions.

```ebnf
document             = { declaration } ;

declaration          = type_declaration
                     | resource_declaration ;

resource_declaration = "resource" identifier
                       "version" integer_literal
                       object_body ;

type_declaration    = "type" identifier
                      ( refined_type
                      | enum_type
                      | object_body ) ;

refined_type         = field_type
                       [ "where" expression ]
                       [ range_constraint ] ;

enum_type            = field_type enumeration ;

object_body          = "{"
                       { object_member }
                       "}" ;

object_member        = field_declaration
                     | nested_object
                     | conditional_declaration ;

nested_object        = identifier object_body
                       [ "optional" ] ;

field_declaration    = identifier ":"
                       field_type
                       { field_modifier }
                       [ default_value ] ;

field_type           = primary_type
                       { list_suffix } ;

primary_type         = identifier
                     | "map" "[" field_type "]" field_type
                     | "(" field_type ")" ;

list_suffix          = "[" [ cardinality ] "]" ;

cardinality          = integer_literal
                     | [ integer_literal ] ":"
                       [ integer_literal ] ;

field_modifier       = range_constraint
                     | length_constraint
                     | enumeration
                     | pattern_constraint
                     | "optional"
                     | "nullable"
                     | "unique" ;

range_constraint     = "range" range_expression ;

range_expression     = value "to" value
                     | value "+"
                     | value "to" "*"
                     | "*" "to" value ;

length_constraint    = "length" integer_literal
                       [ "to" integer_literal ] ;

enumeration          = "oneof" "(" value_list ")" ;

value_list           = value { [ "," ] value } ;

pattern_constraint   = "matches" "(" string_literal ")" ;

default_value        = "=" value ;

conditional_declaration
                     = "if" expression object_body
                       [ "else" object_body ] ;

expression           = logical_expression ;

logical_expression   = comparison_expression
                       { logical_operator comparison_expression } ;

comparison_expression
                     = additive_expression
                       [ comparison_operator additive_expression ]
                       | additive_expression "in" "(" value_list ")" ;

additive_expression  = multiplicative_expression
                       { ( "+" | "-" ) multiplicative_expression } ;

multiplicative_expression
                     = unary_expression
                       { ( "*" | "/" | "%" ) unary_expression } ;

unary_expression     = [ "not" | "-" ] primary_expression ;

primary_expression   = value
                     | field_reference
                     | function_call
                     | "(" expression ")" ;

field_reference     = identifier { "." identifier } ;

function_call       = identifier "(" [ argument_list ] ")" ;

argument_list       = expression { "," expression } ;

logical_operator    = "and" | "or" ;

comparison_operator = "==" | "!="
                    | "<" | "<="
                    | ">" | ">=" ;

value               = string_literal
                    | numeric_literal
                    | boolean_literal
                    | "null"
                    | identifier ;

boolean_literal     = "true" | "false" ;

identifier          = letter { letter | digit | "_" } ;

integer_literal     = digit { digit } ;

numeric_literal     = integer_literal
                    | integer_literal "." integer_literal ;

string_literal      = '"' { character } '"' ;
```

### 13.1 Grammar and parsing rules

The grammar is supplemented by these rules:

1. `map[key]value` is parsed as a map type. The key type must be `string` in v0.2.
2. A list suffix applies to the immediately preceding type expression.
3. A cardinality containing one integer specifies a maximum, with a minimum of zero.
4. In a cardinality range, an omitted lower bound means zero and an omitted upper bound means unbounded.
5. Unquoted identifiers inside `oneof(...)` are literal string values.
6. In a default declaration, an unquoted identifier is interpreted as a string literal when the field type is string or a string-based custom type.
7. `=` is used for default assignment. `==` is used for equality comparisons.
8. Field declarations are separated by whitespace or newlines; semicolons are reserved for comments.
9. Duplicate field declarations in the same object scope are invalid unless explicitly combined through a valid conditional declaration.
10. Implementations must reject ambiguous or unsupported syntax rather than guessing its meaning.

---

## 14. Runtime representation

CSL describes logical resource structures. The control plane is responsible for translating them into its internal representation.

Recommended canonical representations:

| CSL type | Runtime representation |
|---|---|
| `string` | UTF-8 string |
| `bool` | Boolean |
| Integer types | Exact integer |
| `float32`, `float64` | IEEE 754 floating point |
| `decimal` | Arbitrary-precision decimal |
| `bytes` | Integer byte count |
| `duration` | Canonical duration representation |
| `timestamp` | UTC instant with preserved or normalized timezone semantics |
| `date` | ISO 8601 date |
| `time` | ISO 8601 time |
| `uuid`, `ulid` | Canonical string |
| `ip`, `ipv4`, `ipv6` | Canonical address string |
| `cidr` | Canonical network prefix string |
| `mac` | Canonical MAC address string |
| `url`, `uri`, `email` | Validated string |
| `map` | Object with string keys |
| List | Ordered array |
| Object | Named fields |

The wire representation must be stable across implementations.

JSON resource documents should use:

- JSON numbers for integer and floating-point values.
- Canonical decimal strings for `decimal` values to avoid precision loss.
- Integer byte counts for `bytes`.
- Canonical duration strings for `duration`.
- RFC 3339 strings for timestamps.
- Strings for IP addresses, network prefixes, URLs, identifiers, and other string-based specialized types.
- JSON objects for maps and nested objects.
- JSON arrays for lists.

The schema runtime must validate numeric precision and range before converting values into internal representations.

---

## 15. Schema registration and execution

A CSL implementation must provide:

1. A lexer and parser.
2. A schema abstract syntax tree.
3. A type resolver.
4. A schema compiler.
5. A validation engine.
6. A deterministic default-value evaluator.
7. A conditional validation evaluator.
8. A schema registry.
9. Schema version management.
10. Structured validation errors.

Schema registration must validate the entire schema before making it available.

Invalid schemas must not become active.

The runtime should compile registered schemas into an efficient internal representation so resource validation does not require reparsing CSL source for every operation.

Validation errors should identify:

- Resource kind.
- Schema version.
- Field path.
- Expected type or constraint.
- Actual value, where safe to disclose.
- Error category.

Example:

```text
ValidationError {
    kind: "VirtualMachine"
    version: 1
    path: "spec.cpu"
    constraint: "range 1 to 128"
    message: "value must be between 1 and 128"
}
```

Sensitive values must not be included in validation errors.

---

## 16. Explicitly excluded from v0.2

The following features are not part of this version:

- Arbitrary code execution.
- User-defined functions.
- Loops and iteration.
- `switch` statements.
- External service calls during validation.
- Filesystem access during validation.
- Database queries from expressions.
- Dynamic inheritance.
- Implicit schema migrations.
- Implicit resource mutation from conditional declarations.
- Automatic increment fields.
- Runtime evaluation of nondeterministic defaults.
- User-defined map key types.

These features may be considered for later versions, but must not compromise deterministic validation or schema portability.

---

# 17. Examples

## Example 1: VirtualMachine

```text
type domain_name string where matches("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")

type machine_model string oneof(q35 i440fx virt pc)

type disk_size bytes range 10GiB to 1TiB

resource VirtualMachine version 1 {
    metadata {
        name: hostname
        site: string length 1 to 255
        address: ip optional
        labels: map[string]string optional
        annotations: map[string]string optional
    }

    spec {
        cpu: int16 range 1 to 128
        memory: bytes range 512MiB+
        image: string

        networks: domain_name[1:8] optional

        monitored: bool = true
        model: machine_model = q35

        number_of_disks: int8 oneof(1 2 3)
        size_of_disks: disk_size[1:10] optional

        backup {
            enabled: bool = false
            schedule: crontab = "0 0 * * *"
        }

        if spec.monitored == true {
            monitoring {
                endpoint: url
                interval: duration = "30s"
                timeout: duration = "5s"
            }
        }
    }
}
```

Example resource:

```json
{
  "kind": "VirtualMachine",
  "metadata": {
    "name": "web-01",
    "site": "production",
    "address": "192.168.10.20",
    "labels": {
      "environment": "production",
      "team": "platform"
    }
  },
  "spec": {
    "cpu": 4,
    "memory": 8589934592,
    "image": "ubuntu-24.04",
    "networks": [
      "frontend",
      "backend"
    ],
    "monitored": true,
    "model": "q35",
    "number_of_disks": 2,
    "size_of_disks": [
      107374182400,
      536870912000
    ],
    "backup": {
      "enabled": true,
      "schedule": "0 0 * * *"
    },
    "monitoring": {
      "endpoint": "https://monitoring.example.com",
      "interval": "30s",
      "timeout": "5s"
    }
  }
}
```

## Example 2: Database

```text
type database_engine string oneof(postgres mysql mariadb sqlite)

type database_role string oneof(read write admin)

type database_username string where matches("^[a-zA-Z][a-zA-Z0-9_]{2,31}$")

type DatabaseLimits {
    connections: uint16 range 1 to 10000
    memory: bytes range 128MiB+
    cpu: decimal range 0.1 to 128
}

type DatabaseUser {
    username: database_username
    role: database_role
    password: string
}

resource Database version 1 {
    metadata {
        name: hostname
        namespace: string
        labels: map[string]string optional
    }

    spec {
        engine: database_engine
        version: string

        replicas: uint8 = 1
        storage: bytes range 1GiB+

        limits: DatabaseLimits

        users: DatabaseUser[1:100]

        backup {
            enabled: bool = true
            schedule: crontab = "0 2 * * *"
            retention_days: uint16 = 14
        }

        high_availability: bool = false

        if spec.high_availability == true {
            replication {
                mode: string oneof(sync async)
                replicas: uint8 range 2 to 5
            }
        }
    }
}
```

## Example 3: Network configuration

```text
type network_protocol string oneof(tcp udp)

type NetworkPort {
    name: string
    protocol: network_protocol = tcp
    port: port
}

resouce NetworkPolicy version 1 {
    metadata {
        name: hostname
        namespace: string
    }

    spec {
        enabled: bool = true

        source_addresses: cidr[1:64]
        destination_addresses: cidr[1:64]

        ingress: NetworkPort[0:128]
        egress: NetworkPort[0:128]

        allowed_domains: fqdn[0:100] optional
    }
}
```

---

## 18. Future considerations

Potential future additions include:

- Imports and reusable schema packages.
- Namespaces for custom types.
- Schema composition and controlled inheritance.
- More advanced conditional expressions.
- Cross-field validation blocks.
- Explicit migration definitions.
- Additional built-in types for cloud resources, storage, hardware, and Kubernetes-compatible values.
- Custom validation annotations.
- Field-level sensitivity and redaction declarations.
- Read-only and immutable field modifiers.
- Deprecation annotations.
- Documentation annotations.

These additions should remain declarative and deterministic.

---

## 19. Design summary

CSL v0.2 uses a consistent type-oriented syntax:

```text
; Primitive
name: string

; Numeric constraint
cpu: int16 range 1 to 128

; Default
enabled: bool = true

; Optional
description: string optional

; Enumeration
model: string oneof(q35 i440fx virt pc)

; List
networks: string[]

; Bounded list
networks: string[1:8]

; Map
labels: map[string]string

; Map containing lists
groups: map[string]string[1:8]

; Nested object
resources {
    cpu: int16
    memory: bytes
}

; Conditional declaration
if spec.enabled == true {
    monitoring {
        endpoint: url
    }
}
```

The guiding principle is that **types define structure, type suffixes define collection cardinality, constraints define valid values, and declarations define resource behavior**.

CSL should remain small enough to implement consistently across languages and runtimes, while expressive enough to describe the resource kinds needed by a dynamic control plane.