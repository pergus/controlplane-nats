# Controlplane Schema Language (CSL)

## Formal Language Specification - Draft v0.3

**Status:** Working draft\
**Purpose:** Declarative resource schema definition for Controlplane\
**Inputs:** CSL Draft v0.1 and CSL Draft v0.2 supplied for this revision

------------------------------------------------------------------------

## Revision note

This draft consolidates the two supplied drafts. It uses v0.2's resource
model as the current direction: resource-level fields are ordinary
fields, while `desired` and `observed` are optional lifecycle sections.
It restores and retains substantial language features specified in v0.1
that v0.2 omitted or incorrectly described as absent, including reusable
enumerated types, inline enumerations, numeric and boolean enumerators,
nullable fields, pattern constraints, uniqueness constraints, refined
types, expressions, formal grammar, runtime representation, schema
compilation and registration requirements, and the fuller built-in type
inventory.

This is a consolidation, not a claim that every feature is fully
settled. Where the source drafts disagree, this document records the
conflict and identifies the working rule or open decision. It does not
silently treat an omission in v0.2 as an intentional removal.

### Main consolidation decisions

1.  The v0.2 resource model is the canonical model for this draft.
    `metadata` and `spec` are not mandatory or special sections.
    Resource-level fields are ordinary fields. `desired` and `observed`
    are optional sections with lifecycle meaning.
2.  The v0.1 type inventory is retained and combined with v0.2's
    inventory. Duplicate aliases are identified rather than discarded.
3.  Enumerations remain supported both inline and as reusable named
    types. Enumerators may be strings, identifiers representing string
    literals, numbers, or booleans.
4.  `optional` and `nullable` are distinct and both are retained. The
    v0.2 statement that null is not permitted is treated as a
    conflicting proposal, not as an agreed removal.
5.  List cardinality remains part of list type syntax. The v0.1 `unique`
    modifier is retained as a supported proposed constraint.
6.  The v0.1 expression and conditional-validation material is retained,
    with v0.2's clearer desired-state example incorporated.
7.  The v0.1 formal grammar, runtime representation, schema registration
    and execution requirements, and excluded-feature list are restored.
8.  The v0.2 reserved-word list is not adopted as a global list of
    forbidden field names. CSL uses contextual keywords. See Section 3.
9.  `version` is a contextual token in the resource declaration header,
    not a globally reserved identifier. A field named `version` is
    valid.
10. The spelling `resource` is used consistently; occurrences of
    `resouce` in the source drafts are treated as typographical errors.

## 1. Introduction

Controlplane Schema Language (CSL) is a declarative language for
defining resource kinds, their fields, data types, constraints,
defaults, and validation rules.

CSL is designed for a dynamic control plane in which resource kinds and
schema versions can be registered at runtime, without database schema
migrations or application restarts.

The language is intended to be:

-   **Declarative:** schemas describe data and permitted values, not
    application logic.
-   **Strongly typed:** every field has an explicit type.
-   **Compact:** common structures have concise syntax.
-   **Extensible:** users can define reusable custom types.
-   **Deterministic:** validation does not depend on external services
    or arbitrary code.
-   **Versioned:** registered schema versions are immutable.
-   **Patch-friendly:** partial updates modify only specified
    attributes, followed by validation of the complete resulting
    resource.
-   **Portable:** the same schema has consistent meaning across
    implementations.

CSL describes the logical structure and validity of resources. It does
not define authorization, storage layout, API endpoints, reconciliation
logic, controller implementation, or message-broker behavior.

## 2. Resource declarations and lifecycle sections

A resource kind is declared using `resource`, followed by its name, a
schema version, and a body.

``` csl
resource VirtualMachine version 1 {
    name: hostname
    namespace: string
    labels: map[string]string optional
    annotations: map[string]string optional

    desired {
        cpu: int16 range 1 to 128
        memory: bytes range 512MiB+
        image: string
    }

    observed {
        phase: string oneof(pending provisioning running stopped failed)
        ready: bool = false
        ip_address: ip optional
        node: hostname optional
    }
}
```

Resource names conventionally use PascalCase. Keywords, field names, and
built-in type names conventionally use lowercase. Identifiers are
case-sensitive.

A resource declaration may contain:

-   Resource-level fields.
-   An optional `desired` section.
-   An optional `observed` section.
-   Nested objects and conditional declarations within permitted object
    scopes.

A resource need not declare either lifecycle section. It may consist
entirely of resource-level fields.

### 2.1 Resource-level fields

Fields declared directly inside a resource are ordinary fields. CSL does
not require `name`, `namespace`, `labels`, `annotations`, `tenant`, or
any other conventional metadata field.

``` csl
resource Customer version 1 {
    customer_id: uuid
    tenant: string
    display_name: string
    labels: map[string]string optional
}
```

CSL does not assign built-in identity or uniqueness semantics to fields
such as `name`, `namespace`, `tenant`, or `customer_id`. A resource may
define business identity using any combination of fields, but
enforcement of uniqueness or naming policies is a runtime concern unless
a future CSL feature explicitly defines it.

### 2.2 Runtime identity

Every resource receives a runtime-generated, globally unique `uid`.

The `uid`:

-   Is generated by Controlplane.
-   Is immutable.
-   Is used internally to identify a resource.
-   Does not need to be declared in CSL.
-   Cannot be supplied or changed by clients through ordinary resource
    operations.

Runtime identity is independent of the resource schema.

### 2.3 Desired state

The optional `desired` section describes the state requested by a user,
another system, or an authorized API client.

``` csl
desired {
    cpu: int16 range 1 to 128
    memory: bytes range 512MiB+
    image: string
}
```

Desired state represents intent. Controllers may observe changes to it
and reconcile external systems toward that intent. The runtime must
preserve the distinction between desired state and observed state.

### 2.4 Observed state

The optional `observed` section describes state reported by controllers
or other authorized system components.

``` csl
observed {
    phase: string oneof(pending provisioning running stopped failed)
    ready: bool = false
    ip_address: ip optional
    node: hostname optional
}
```

Observed state represents information about the actual resource or its
reconciliation progress, not user intent. CSL does not prescribe how
many controllers may update observed state.

### 2.5 Section semantics

`desired` and `observed` are recognized lifecycle section names when
used as section declarations. They are not authorization declarations.
Authorization policy determines which identities may read or modify
fields or sections, and is enforced independently of CSL.

`desired` and `observed` are not globally forbidden field identifiers.
For example, `desired: string` is an ordinary field declaration. The
form `desired { ... }` introduces the lifecycle section.

## 3. Lexical conventions, identifiers, and keywords

### 3.1 Comments

A semicolon begins a comment extending to the end of the line.

``` csl
; This is a comment.
cpu: int16 range 1 to 128 ; CPU allocation
```

### 3.2 Identifiers

Identifiers begin with a letter or underscore and may contain letters,
digits, and underscores.

Examples:

``` text
VirtualMachine
machine_model
number_of_disks
version
desired
optional
```

Identifiers are case-sensitive. The grammar determines whether an
identifier is interpreted as a field name, type name, resource name, or
contextual keyword.

### 3.3 Contextual keywords --- not globally reserved field names

The v0.2 draft listed `resource`, `type`, `desired`, `observed`, `if`,
`oneof`, `optional`, `range`, `length`, `map`, and `to` as "reserved
words". That wording is too broad unless it states what positions they
are reserved in. This draft does **not** make those words globally
unavailable as field names.

CSL uses contextual keywords: a word has keyword meaning only in a
grammar position where that construct is expected. In particular:

  -----------------------------------------------------------------------
  Word                    Keyword meaning applies Field-name status
                          in                      
  ----------------------- ----------------------- -----------------------
  `resource`              Top-level resource      May be used as a field
                          declaration, when       name when followed by
                          followed by a resource  `:`
                          name                    

  `type`                  Top-level custom type   May be used as a field
                          declaration             name when followed by
                                                  `:`

  `version`               Resource declaration    Valid as an ordinary
                          header after the        field name
                          resource name           

  `desired`               Lifecycle section       Valid as a field name
                          declaration when        when followed by `:`
                          followed by an object   
                          body `{ ... }`          

  `observed`              Lifecycle section       Valid as a field name
                          declaration when        when followed by `:`
                          followed by an object   
                          body `{ ... }`          

  `if`                    Conditional declaration May be used as a field
                          in an object body       name when followed by
                                                  `:` if the parser can
                                                  distinguish the
                                                  declaration

  `oneof`                 Enumeration constraint  May be used as a field
                          after a type expression name when followed by
                                                  `:`

  `optional`              Field or nested-object  May be used as a field
                          modifier in modifier    name when followed by
                          position                `:`

  `nullable`              Field modifier in       May be used as a field
                          modifier position       name when followed by
                                                  `:`

  `range`                 Range constraint after  May be used as a field
                          a type expression       name when followed by
                                                  `:`

  `length`                Length constraint after May be used as a field
                          a type expression       name when followed by
                                                  `:`

  `map`                   Map type constructor    May be used as a field
                          when followed by `[` in name when followed by
                          a type expression       `:`

  `to`                    Range-expression        May be used as a field
                          separator               name when followed by
                                                  `:`

  `where`                 Refined-type predicate  May be used as a field
                          introducer              name when followed by
                                                  `:`

  `matches`               Pattern constraint or   May be used as a field
                          approved pure-function  name when followed by
                          call                    `:`

  `unique`                Collection uniqueness   May be used as a field
                          modifier in modifier    name when followed by
                          position                `:`

  `and`, `or`, `not`,     Expression operators    May be used as a field
  `in`                                            name when followed by
                                                  `:`
  -----------------------------------------------------------------------

This table describes parser context, not a list of globally forbidden
identifiers. A parser must use syntactic position and lookahead to
distinguish declarations from modifiers. Implementations must reject
genuinely ambiguous or malformed input rather than silently reinterpret
it.

The words `true`, `false`, and `null` are literals in value-expression
positions. Whether they may be used as field names is a lexical/parser
decision that must be consistent; this draft recommends allowing them as
field names only when unambiguously followed by `:`.

### 3.4 Strings

Strings use double quotes.

``` csl
name: string = "production"
```

Supported escapes:

  Escape     Meaning
  ---------- --------------------
  `\"`       Double quote
  `\\`       Backslash
  `\n`       Newline
  `\r`       Carriage return
  `\t`       Tab
  `\uXXXX`   Unicode code point

The exact treatment of surrogate pairs and invalid Unicode escape
sequences must be specified by the parser implementation profile.

### 3.5 Numeric literals and units

Integer and decimal literals are supported.

``` csl
cpu: int16 = 4
ratio: decimal = 1.25
memory: bytes = 8GiB
timeout: duration = "30s"
```

Numeric literals may include units when the expected type supports them.

Supported byte units:

-   Decimal: `B`, `KB`, `MB`, `GB`, `TB`, `PB`.
-   Binary: `KiB`, `MiB`, `GiB`, `TiB`, `PiB`.

Byte values are represented internally as non-negative integer byte
counts. The exact treatment of fractional byte quantities must be
rejected or explicitly defined; implementations must not round silently.

Duration values use duration strings with explicit units, such as
`"30s"`, `"5m"`, or `"2h"`. The accepted duration grammar and
canonicalization rules must be consistent across implementations.

### 3.6 Whitespace and separators

Whitespace separates tokens. Commas are optional between values in
`oneof(...)`; commas are not generally required or used as field
terminators. Braces define object and conditional scopes. Field
declarations are separated by whitespace or newlines.

## 4. Built-in types

The following inventory consolidates the types present in either
supplied draft. Inclusion in this inventory preserves the proposal; it
does not imply that every implementation detail is already fully
standardized.

### 4.1 Primitive and general types

  Type        Description
  ----------- ---------------------------------------
  `string`    UTF-8 string
  `bool`      Boolean
  `boolean`   Alias for `bool`
  `any`       Any supported CSL value
  `json`      Arbitrary valid JSON-compatible value

`any` accepts any supported CSL value, while `json` specifically
represents JSON-compatible data.

### 4.2 Integer types

  Type       Description
  ---------- -------------------------
  `int`      Alias for `int64`
  `int8`     Signed 8-bit integer
  `int16`    Signed 16-bit integer
  `int32`    Signed 32-bit integer
  `int64`    Signed 64-bit integer
  `uint`     Alias for `uint64`
  `uint8`    Unsigned 8-bit integer
  `uint16`   Unsigned 16-bit integer
  `uint32`   Unsigned 32-bit integer
  `uint64`   Unsigned 64-bit integer

Integer types reject fractional values and values outside their
representable ranges.

### 4.3 Floating-point and decimal types

  Type        Description
  ----------- --------------------------------
  `float32`   IEEE 754 32-bit floating point
  `float64`   IEEE 754 64-bit floating point
  `float`     Alias for `float64`
  `decimal`   Arbitrary-precision decimal
  `number`    General numeric value

`decimal` should use an exact representation rather than binary
floating-point semantics and is recommended for financial and
precision-sensitive values. `number` is a convenience type and should
not be used where exact numeric representation is required. NaN and
infinity are invalid unless a future specification explicitly permits
them.

### 4.4 Binary and size types

  Type       Description
  ---------- ----------------------------
  `bytes`    Non-negative byte quantity
  `binary`   Binary data
  `base64`   Base64-encoded data
  `hex`      Hexadecimal-encoded data

`bytes` represents a numeric size, whereas `binary` represents actual
binary content. `base64` and `hex` represent encoded textual forms.

### 4.5 Date and time types

  Type          Description
  ------------- -----------------------------
  `date`        Calendar date
  `time`        Time of day
  `timestamp`   Date and time with timezone
  `datetime`    Alias for `timestamp`
  `duration`    Time interval
  `timezone`    Timezone identifier

Canonical representations:

-   `date`: ISO 8601 date, such as `2026-11-07`.
-   `time`: ISO 8601 time.
-   `timestamp`: RFC 3339 timestamp with timezone.
-   `duration`: duration string with explicit units; canonical form
    remains to be finalized.
-   `timezone`: IANA timezone identifier, such as `Europe/Stockholm`.

The exact normalization policy for timestamp offsets and timezone
preservation remains an open specification detail.

### 4.6 Identifier types

  Type      Description
  --------- -----------------
  `uuid`    UUID identifier
  `uuid4`   UUID version 4
  `uuid7`   UUID version 7
  `ulid`    ULID identifier

Identifier types are represented as strings in resource documents and
should use canonical textual forms.

### 4.7 Networking types

  Type          Description
  ------------- -----------------------------
  `ip`          IPv4 or IPv6 address
  `ipv4`        IPv4 address
  `ipv6`        IPv6 address
  `cidr`        IPv4 or IPv6 network prefix
  `ipv4_cidr`   IPv4 network prefix
  `ipv6_cidr`   IPv6 network prefix
  `mac`         MAC address
  `hostname`    Hostname
  `domain`      Domain name
  `fqdn`        Fully qualified domain name
  `dns`         DNS name
  `url`         Absolute URL
  `uri`         URI
  `email`       Email address
  `port`        TCP/UDP port number
  `protocol`    Network protocol identifier

Additional semantics:

-   `ip` accepts either IPv4 or IPv6.
-   `ipv4` and `ipv6` enforce their respective address formats.
-   `cidr` accepts a network address with a prefix length.
-   `ipv4_cidr` and `ipv6_cidr` enforce the respective address family.
-   `mac` accepts a valid MAC address.
-   `hostname` follows hostname label rules.
-   `domain` represents a valid domain name.
-   `fqdn` requires a fully qualified domain name.
-   `dns` accepts a valid DNS name.
-   `url` requires an absolute URL with a valid scheme.
-   `uri` accepts a syntactically valid URI.
-   `email` validates email address syntax, not mailbox existence.
-   `port` is an integer from 0 to 65535.
-   `protocol` accepts registered protocol identifiers and explicitly
    defined numeric protocol values.

DNS validation must not require an external DNS lookup.

### 4.8 Text and format types

  Type             Description
  ---------------- ----------------------------------------
  `alpha`          Alphabetic characters
  `alphanumeric`   Alphabetic and numeric characters
  `numeric`        Numeric characters represented as text
  `hexadecimal`    Hexadecimal text
  `path`           Generic path
  `filepath`       File path
  `directory`      Directory path
  `crontab`        Cron expression

Path types perform syntactic validation only. They must not access the
filesystem or verify that a path exists. `crontab` validates expression
syntax; it does not execute schedules.

### 4.9 Cryptographic and certificate types

  Type            Description
  --------------- -----------------------------
  `certificate`   X.509 certificate
  `csr`           Certificate signing request
  `pem`           PEM-encoded data
  `public_key`    Public key
  `private_key`   Private key

These types validate encoding and, where applicable, structural
validity. They do not imply that a certificate is trusted, a key is
authorized, or a signature is valid.

A private-key field should be treated as sensitive by the runtime and
must not be exposed through ordinary resource reads, logs, or event
payloads without an explicit security policy. The precise redaction and
storage behavior is a runtime/security concern, not defined by the type
alone.

## 5. Compound types

### 5.1 Lists

Lists are declared by appending square brackets to a type.

``` csl
networks: string[]
networks: string[1:8]
```

Supported forms:

  Syntax           Meaning
  ---------------- ------------------------
  `string[]`       Zero or more elements
  `string[8]`      Zero to eight elements
  `string[1:8]`    One to eight elements
  `string[3:10]`   Three to ten elements
  `string[5:5]`    Exactly five elements
  `string[1:]`     At least one element
  `string[:]`      Zero or more elements

A single integer specifies the maximum number of elements. An omitted
lower bound defaults to zero. An omitted upper bound means unlimited,
subject to runtime resource limits. Cardinality constrains the number of
elements, not the serialized size.

### 5.2 Nested lists

Lists can contain other lists.

``` csl
matrix: int16[][]
matrix: int16[1:8][1:8]
```

Cardinality applies to the immediately preceding list constructor. The
grammar and examples in v0.1 describe suffix ordering inconsistently
(including an example whose prose reverses which suffix is outermost).
This draft does not silently standardize that ambiguity: the parser
specification must define suffix binding unambiguously before
implementation. For readability, deeply nested lists should generally
use named custom types.

### 5.3 Maps

Maps use Go-like generic syntax:

``` csl
labels: map[string]string
```

General form:

``` csl
map[key_type]value_type
```

The original v0.1 draft initially restricted map keys to strings; v0.2
proposed scalar key types with stable equality and serialization. This
draft preserves both proposals as an unresolved design choice. **Working
compatibility rule:** implementations must support string keys. Support
for additional scalar key types is optional until the language version
explicitly standardizes it.

Examples:

``` csl
labels: map[string]string
resources: map[string]ResourceLimits
groups: map[string]string[]
configuration: map[string]map[string]string
ports: map[string]uint16
addresses: map[string]ip
networks: map[string]ip[1:8]
```

Map values may themselves be lists or maps. Map keys must be unique;
duplicate keys in a resource document are invalid. If non-string map
keys are later standardized, their equality and serialization semantics
must be defined.

### 5.4 Objects

Objects contain named fields.

``` csl
ResourceLimits {
    cpu: int16
    memory: bytes
}
```

Objects may be nested:

``` csl
desired {
    cpu: int16
    memory: bytes

    backup {
        enabled: bool
        schedule: crontab
    }
}
```

Object fields follow the same declaration, type, constraint, default,
optionality, nullability, and validation rules as resource fields.

## 6. Custom types

Custom types allow users to define reusable types with additional
restrictions.

### 6.1 Refined types

A refined type adds constraints to an existing type.

``` csl
type domain_name string where matches("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
type positive_integer int64 range 1+
```

A refined type inherits the validation rules of its underlying type and
applies its additional constraints.

### 6.2 Enumerated types

An enumeration defines a finite set of permitted values. Enumerations
are supported both inline and as reusable named types. They were present
in v0.1 and remain part of this consolidated draft.

``` csl
type machine_model string oneof(q35 i440fx virt pc)
```

Equivalent syntax with commas is also accepted:

``` csl
type machine_model string oneof(q35, i440fx, virt, pc)
```

Enumeration values may be identifiers, strings, numbers, or booleans:

``` csl
type database_engine string oneof(postgres mysql sqlite)
type replica_count int8 oneof(1 2 3 5)
type enabled_state bool oneof(true false)
```

Inline enumerations are also supported:

``` csl
model: string oneof(q35 i440fx virt pc)
phase: string oneof("not started" "in progress" completed)
```

For string enumerations, unquoted identifiers inside `oneof(...)` are
literal string values, not variable references. Quoted strings are
supported, including values containing spaces. Commas between
enumeration values are optional.

Enumeration values must be compatible with the underlying type.
Duplicate values after canonicalization are invalid. The exact equality
and canonicalization rules for numeric and specialized values must be
consistent across implementations.

### 6.3 Named object types

Reusable object types can be declared independently.

``` csl
type ResourceLimits {
    cpu: int16 range 1 to 128
    memory: bytes range 512MiB+
}

type DatabaseUser {
    username: string
    role: string oneof(read write admin)
    password: string
}
```

They can then be referenced by other types or resources:

``` csl
resource VirtualMachine version 1 {
    desired {
        limits: ResourceLimits
    }
}
```

### 6.4 Type restrictions

Custom types:

-   Must have a valid underlying type or object definition.
-   Must not redefine built-in types.
-   Must not contain circular type references.
-   Must be deterministic.
-   Must not execute arbitrary code.
-   Must be resolved when the schema is registered.
-   Are scoped to their schema namespace unless the runtime explicitly
    supports shared type namespaces.

## 7. Field declarations

A field declaration has the following general form:

``` csl
field_name: TYPE [constraints and modifiers] [= default]
```

Examples:

``` csl
cpu: int16 range 1 to 128
memory: bytes range 512MiB+
hostname: hostname
description: string optional
description: string nullable
enabled: bool = true
```

Field names must be unique within their containing object or section,
subject to the conditional-branch compatibility rules in Section 11.
Unknown fields are rejected unless a future language extension
explicitly permits them.

### 7.1 Required fields

Fields are required by default.

``` csl
image: string
```

A required field must be present when creating a resource and must
remain present in the complete resulting resource after an update.

### 7.2 Optional fields

The `optional` modifier allows a field to be omitted.

``` csl
description: string optional
```

An omitted optional field is not automatically assigned a value unless
it has a default.

### 7.3 Nullable fields

The `nullable` modifier allows an explicitly supplied `null` value.

``` csl
description: string nullable
description: string optional nullable
```

Optionality and nullability are independent:

-   Required and non-nullable: the field must be present and cannot be
    null.
-   Optional and non-nullable: the field may be absent but cannot be
    null when present.
-   Required and nullable: the field must be present but may contain
    null.
-   Optional and nullable: the field may be absent or explicitly null.

This retains v0.1's explicit nullable semantics. v0.2 proposed that
explicit null values should not be permitted in that version. That is a
source-draft conflict; this consolidated draft keeps `nullable` as a
proposed language feature rather than silently deleting it. Whether
nullable is included in the first implementation profile remains an open
decision.

### 7.4 Default values

Defaults are specified using `=`.

``` csl
enabled: bool = true
model: machine_model = q35
replicas: int16 = 1
```

Defaults are applied during resource creation when the field is omitted.
A defaulted field is present in the resulting resource.

Defaults are not reapplied during updates. Removing an optional field
does not automatically restore its creation default. Default values must
be valid instances of their declared types. A default is evaluated
deterministically and must not depend on external services, current
time, or arbitrary code.

An unquoted identifier in a default is interpreted as a string literal
when the field type is `string` or a string-based custom type. In other
contexts it is resolved according to the expected type, such as an enum
member. The parser and type checker must reject unresolved or ambiguous
defaults.

## 8. Constraints

Constraints restrict the values a field may contain. A field may have
multiple compatible constraints. The grammar must define a canonical
modifier order or permit modifiers in any order while rejecting
duplicates and contradictions.

### 8.1 Numeric ranges

``` csl
cpu: int16 range 1 to 128
memory: bytes range 512MiB+
temperature: decimal range -40 to 125
```

Supported forms:

``` text
range 1 to 128
range 1+
range 1 to *
range * to 128
```

`*` represents an unbounded endpoint. Ranges are inclusive. Bounds must
be compatible with the field's underlying type and must not define an
empty range.

### 8.2 String length

``` csl
hostname: string length 1 to 63
description: string length 0 to 1024
```

Length is measured in Unicode code points, not UTF-8 bytes, as proposed
in v0.1. The implementation profile must define Unicode version and
treatment of invalid UTF-8 consistently.

### 8.3 Pattern matching

``` csl
username: string matches("^[a-z][a-z0-9_]{2,31}$")
```

Patterns use a defined regular-expression dialect. The runtime must use
the same dialect consistently across implementations. Pattern matching
is validation only and must not execute arbitrary code. The exact regex
dialect and resource limits remain to be finalized.

### 8.4 Enumerations

A field can define permitted values inline or reference a named enum
type.

``` csl
model: string oneof(q35 i440fx virt pc)

type machine_model string oneof(q35 i440fx virt pc)
model: machine_model
```

See Section 6.2. Enumerations are not absent from CSL merely because a
particular draft section or inventory omitted them.

### 8.5 Collection cardinality

Collection cardinality is part of the type declaration.

``` csl
networks: string[1:8]
ports: port[1:16]
```

The syntax is described in Section 5.1. Separate `min_items` and
`max_items` modifiers are not part of the canonical syntax in these
drafts.

### 8.6 Uniqueness

The v0.1 `unique` modifier is retained as a proposed list constraint:

``` csl
networks: string[1:8] unique
ports: port[1:16] unique
```

Uniqueness is evaluated using the canonical equality semantics of the
element type. Whether `unique` applies to map values or nested
collections is not established by the source drafts and remains open.
Map keys are unique by definition.

## 9. Expressions

CSL supports a restricted expression language for validation and
conditional declarations. Expressions must be deterministic and
side-effect-free.

### 9.1 Comparison operators

  Operator   Meaning
  ---------- -----------------------
  `==`       Equal
  `!=`       Not equal
  `<`        Less than
  `<=`       Less than or equal
  `>`        Greater than
  `>=`       Greater than or equal

### 9.2 Logical operators

The logical operators are `and`, `or`, and `not`.

``` csl
if desired.enabled == true and desired.replicas > 1 {
    ...
}
```

### 9.3 Membership

The `in` operator checks whether a value belongs to a set.

``` csl
if desired.model in (q35 i440fx) {
    ...
}
```

### 9.4 Arithmetic

The expression language supports:

``` text
+  -  *  /  %
```

Arithmetic is restricted to compatible numeric types. Division by zero
is invalid. Overflow and precision behavior must be deterministic and
defined by operand types.

### 9.5 Field references

Fields are referenced using dot notation.

``` csl
desired.cpu
desired.memory
tenant
```

References are resolved against the resource being validated. A
reference to an unknown field is a schema registration error. For
expressions nested inside a resource section, the canonical
root-reference convention must be consistent. The v0.1 examples use
`spec.cpu`; v0.2 examples use `desired.backup_enabled`. This draft
adopts section-qualified references such as `desired.cpu` and ordinary
resource-level references such as `tenant`.

### 9.6 Functions

The v0.1 specification permits a small set of built-in pure functions,
including:

``` csl
matches(value, pattern)
```

Additional functions may be standardized in later versions. User-defined
functions and arbitrary code execution are not supported. Function
availability, typing, complexity limits, and deterministic behavior must
be specified by the language version.

## 10. Conditional validation

Conditional declarations allow fields to become required or have
different validation rules depending on other resource values.

### 10.1 Syntax

``` csl
if expression {
    ...
} else {
    ...
}
```

The `else` branch is optional.

### 10.2 Example

``` csl
resource Database version 1 {
    desired {
        backup_enabled: bool = false

        if desired.backup_enabled {
            backup_schedule: crontab
            backup_retention: uint16 range 1 to 365
        }
    }
}
```

The v0.1 form with an explicit comparison remains valid:

``` csl
if desired.backup_enabled == true {
    ...
}
```

### 10.3 Semantics

-   Only the active branch determines which conditional fields are
    required.
-   Fields in inactive branches are not required.
-   If an inactive-branch field is supplied, it must still conform to
    its declared type and constraints.
-   Conditional declarations do not automatically create, delete, or
    modify fields.
-   Conditional validation is evaluated against the complete resulting
    resource.
-   Conditions must be deterministic and must not depend on external
    state.
-   When multiple branches declare the same field, their definitions
    must be compatible; otherwise schema registration fails.
-   A field declared only in one branch is conditionally required when
    that branch is active, unless it is optional or has a default.

The scope and permitted location of conditional declarations, especially
inside nested objects and lifecycle sections, must be defined
consistently by the grammar.

## 11. Resource validation and updates

CSL defines validation of complete resource state. The runtime must
support partial updates.

### 11.1 Creation

When creating a resource, the runtime must:

1.  Resolve the resource kind and schema version.
2.  Reject unknown fields.
3.  Apply defaults to omitted fields.
4.  Validate primitive and compound types.
5.  Validate field constraints.
6.  Evaluate conditional declarations.
7.  Validate required fields.
8.  Assign system-managed attributes.
9.  Persist the resource and its creation event according to the control
    plane's transaction model.

### 11.2 Partial updates

Updates are partial by default.

Given an existing resource:

``` text
desired {
    cpu: 4
    memory: 8589934592
    image: "ubuntu-24.04"
}
```

A patch containing:

``` text
desired {
    cpu: 8
}
```

changes only `cpu`. Existing `memory` and `image` values remain
unchanged.

The runtime must:

1.  Load the current resource.
2.  Apply the supplied patch to the existing resource.
3.  Enforce authorization on the requested changes (authorization is
    outside CSL, but must be enforced by the runtime).
4.  Validate the complete resulting resource.
5.  Reject the update if any constraint is violated.
6.  Persist the update atomically.
7.  Increment `resource_version`.
8.  Increment `generation` when desired state changes.

The runtime must not interpret an omitted field as deletion, and must
not replace unspecified fields with defaults or zero values.

### 11.3 Removing fields

Field removal must be explicit. The update API must provide an
unambiguous removal operation, such as a dedicated `REMOVE` instruction
or a structured patch deletion operation.

Removing a required field is invalid. Removing an optional field is
permitted. Removing a field does not reapply its creation default. For a
nullable field, setting the field to `null` is distinct from removing
it.

### 11.4 Unknown fields

Unknown fields are rejected by default. This prevents misspelled or
unsupported fields from being silently accepted. A future extension may
support explicitly declared extension fields.

## 12. Schema versioning

Every resource kind has an explicit schema version.

``` csl
resource VirtualMachine version 1 {
    desired {
        cpu: int16 range 1 to 128
    }
}
```

A new schema version is registered as a separate immutable definition:

``` csl
resource VirtualMachine version 2 {
    desired {
        cpu: int16 range 1 to 256
    }
}
```

Rules:

-   Registered schema versions are immutable.
-   Existing resources retain their recorded schema version until
    explicitly migrated.
-   Schema registration must reject invalid definitions.
-   Schema updates must not silently change existing resource
    interpretation.
-   Migrations must be explicit and auditable.
-   The control plane must be able to identify the schema version used
    to validate a resource.
-   A schema registry should support retrieving a kind's available
    versions and identifying its active version.

Migration between versions is a runtime concern and is not executable
CSL functionality.

## 13. System-managed fields

The runtime maintains internal fields independently of CSL. These fields
are not declared by resource authors and cannot be overwritten through
ordinary resource updates.

  Field                Purpose
  -------------------- -------------------------------------
  `uid`                Immutable resource identifier
  `kind`               Resource kind
  `schema_version`     Schema version
  `resource_version`   Version of the stored resource
  `generation`         Version of desired state
  `created_at`         Creation timestamp
  `updated_at`         Last modification timestamp
  `deleted_at`         Deletion timestamp, when applicable

`generation` increments when desired state changes. `resource_version`
changes whenever the stored resource changes, including resource-level
fields, desired state, or observed state. The control plane assigns and
updates these fields.

The exact representation and exposure of system-managed fields in API
responses is a runtime/API concern. Their names are not declarations
that users must include in CSL.

## 14. Formal grammar

The following EBNF is a consolidated core grammar, not yet a complete
parser conformance suite. It restores the grammar work from v0.1 and
incorporates v0.2's resource-level fields and lifecycle sections.
Implementations must reject ambiguous or unsupported syntax rather than
guessing.

``` ebnf
document                 = { declaration } ;

declaration              = type_declaration
                         | resource_declaration ;

resource_declaration     = "resource" identifier
                           "version" integer_literal
                           resource_body ;

resource_body            = "{"
                           { resource_member }
                           "}" ;

resource_member          = field_declaration
                         | lifecycle_section
                         | nested_object
                         | conditional_declaration ;

lifecycle_section        = ("desired" | "observed") object_body ;

type_declaration         = "type" identifier
                           ( refined_type
                           | enum_type
                           | object_body ) ;

refined_type             = field_type
                           [ "where" expression ]
                           [ range_constraint ] ;

enum_type                = field_type enumeration ;

object_body              = "{"
                           { object_member }
                           "}" ;

object_member            = field_declaration
                         | nested_object
                         | conditional_declaration ;

nested_object            = identifier object_body
                           [ "optional" ] ;

field_declaration        = identifier ":"
                           field_type
                           { field_modifier }
                           [ default_value ] ;

field_type               = primary_type
                           { list_suffix } ;

primary_type             = identifier
                         | "map" "[" field_type "]" field_type
                         | "(" field_type ")" ;

list_suffix              = "[" [ cardinality ] "]" ;

cardinality              = integer_literal
                         | [ integer_literal ] ":"
                           [ integer_literal ] ;

field_modifier           = range_constraint
                         | length_constraint
                         | enumeration
                         | pattern_constraint
                         | "optional"
                         | "nullable"
                         | "unique" ;

range_constraint         = "range" range_expression ;

range_expression         = value "to" value
                         | value "+"
                         | value "to" "*"
                         | "*" "to" value ;

length_constraint       = "length" integer_literal
                          [ "to" integer_literal ] ;

enumeration              = "oneof" "(" value_list ")" ;

value_list               = value { [ "," ] value } ;

pattern_constraint       = "matches" "(" string_literal ")" ;

default_value            = "=" value ;

conditional_declaration  = "if" expression object_body
                           [ "else" object_body ] ;

expression               = logical_expression ;

logical_expression       = comparison_expression
                           { logical_operator comparison_expression } ;

comparison_expression    = additive_expression
                           [ comparison_operator additive_expression
                           | "in" "(" value_list ")" ] ;

additive_expression      = multiplicative_expression
                           { ( "+" | "-" ) multiplicative_expression } ;

multiplicative_expression
                         = unary_expression
                           { ( "*" | "/" | "%" ) unary_expression } ;

unary_expression         = [ "not" | "-" ] primary_expression ;

primary_expression       = value
                         | field_reference
                         | function_call
                         | "(" expression ")" ;

field_reference          = identifier { "." identifier } ;

function_call            = identifier "(" [ argument_list ] ")" ;

argument_list            = expression { "," expression } ;

logical_operator         = "and" | "or" ;

comparison_operator      = "==" | "!="
                         | "<" | "<="
                         | ">" | ">=" ;

value                    = string_literal
                         | numeric_literal
                         | boolean_literal
                         | "null"
                         | identifier ;

boolean_literal          = "true" | "false" ;

identifier               = ( letter | "_" )
                           { letter | digit | "_" } ;

integer_literal          = digit { digit } ;

numeric_literal          = integer_literal
                         | integer_literal "." integer_literal ;

string_literal           = '"' { character } '"' ;
```

### 14.1 Grammar and parsing rules

1.  `map[key]value` is parsed as a map type. String keys are required
    for the baseline compatibility profile; additional scalar key types
    remain a design decision.
2.  A list suffix applies to a list type constructor. The binding and
    nesting order of multiple suffixes must be explicitly defined by the
    parser specification.
3.  A cardinality containing one integer specifies a maximum, with a
    minimum of zero.
4.  In a cardinality range, an omitted lower bound means zero and an
    omitted upper bound means unbounded.
5.  Unquoted identifiers inside `oneof(...)` are literal string values
    when the underlying type is string or a string-based custom type.
6.  In a default declaration, an unquoted identifier is interpreted
    according to the expected field type; for string fields it is a
    string literal, and for enum fields it must resolve to a permitted
    enum value.
7.  `=` is used for default assignment. `==` is used for equality
    comparisons.
8.  Field declarations are separated by whitespace or newlines;
    semicolons begin comments and are not field terminators.
9.  Duplicate field declarations in the same object scope are invalid
    unless they are compatible declarations across conditional branches.
10. The parser must distinguish contextual keywords from identifiers
    using grammar position. In particular, `version` is a keyword in the
    resource header, not a globally reserved field name.
11. `desired` and `observed` introduce lifecycle sections when followed
    by an object body; when followed by `:`, they are ordinary field
    names.
12. Implementations must reject ambiguous or unsupported syntax rather
    than guessing its meaning.

The EBNF above is a working grammar. It does not yet fully formalize
every lexical detail, modifier-order rule, nested-list binding rule, or
all conditional-scope restrictions. A parser conformance suite is
required before declaring the grammar final.

## 15. Runtime representation

CSL describes logical resource structures. Controlplane is responsible
for translating them into its internal representation.

Recommended canonical representations:

  CSL type                Runtime representation
  ----------------------- ---------------------------------------------
  `string`                UTF-8 string
  `bool`                  Boolean
  Integer types           Exact integer
  `float32`, `float64`    IEEE 754 floating point
  `decimal`               Arbitrary-precision decimal
  `bytes`                 Integer byte count
  `duration`              Canonical duration representation
  `timestamp`             UTC instant with defined timezone semantics
  `date`                  ISO 8601 date
  `time`                  ISO 8601 time
  `uuid`, `ulid`          Canonical string
  `ip`, `ipv4`, `ipv6`    Canonical address string
  `cidr`                  Canonical network prefix string
  `mac`                   Canonical MAC address string
  `url`, `uri`, `email`   Validated string
  `map`                   Object/map with canonical keys
  List                    Ordered array
  Object                  Named fields

The wire representation must be stable across implementations. JSON
resource documents should use:

-   JSON numbers for integer and floating-point values, subject to exact
    range and precision validation.
-   Canonical decimal strings for `decimal` values to avoid precision
    loss.
-   Integer byte counts for `bytes`.
-   Canonical duration strings for `duration`.
-   RFC 3339 strings for timestamps.
-   Strings for IP addresses, network prefixes, URLs, identifiers, and
    other string-based specialized types.
-   JSON objects for maps and nested objects.
-   JSON arrays for lists.
-   Explicit JSON `null` only where the field is declared `nullable`.

The schema runtime must validate numeric precision and range before
converting values into internal representations. The exact JSON encoding
of `binary`, `any`, and `json` values, and of non-string map keys if
supported, remains to be finalized.

## 16. Schema registration and execution

A CSL implementation must provide:

1.  A lexer and parser.
2.  A schema abstract syntax tree.
3.  A type resolver.
4.  A schema compiler.
5.  A validation engine.
6.  A deterministic default-value evaluator.
7.  A conditional validation evaluator.
8.  A schema registry.
9.  Schema version management.
10. Structured validation errors.

Schema registration must validate the entire schema before making it
available. Invalid schemas must not become active. The runtime should
compile registered schemas into an efficient internal representation so
resource validation does not require reparsing CSL source for every
operation.

Validation errors should identify:

-   Resource kind.
-   Schema version.
-   Field path.
-   Expected type or constraint.
-   Actual value, where safe to disclose.
-   Error category.

Example:

``` text
ValidationError {
    kind: "VirtualMachine"
    version: 1
    path: "desired.cpu"
    constraint: "range 1 to 128"
    message: "value must be between 1 and 128"
}
```

Sensitive values must not be included in validation errors.

## 17. Explicit non-goals and excluded functionality

CSL does not define:

-   Authentication.
-   Authorization policies, role definitions, or user assignments.
-   Storage engines or database schemas.
-   Event brokers.
-   API routing.
-   Controller implementation.
-   Reconciliation scheduling.
-   Arbitrary functions or scripts.
-   Resource identity conventions such as mandatory `name` or
    `namespace`.
-   Implicit schema migrations.
-   External service calls during validation.
-   Filesystem access during validation.
-   Database queries from expressions.
-   Dynamic inheritance.
-   Implicit resource mutation from conditional declarations.
-   Automatic increment fields.
-   Runtime evaluation of nondeterministic defaults.
-   User-defined map key types in the baseline compatibility profile.

The following are also excluded from the current language proposal
unless introduced through an explicitly versioned extension:

-   Loops and iteration.
-   `switch` statements.
-   Arbitrary executable code.
-   User-defined functions.

These exclusions preserve deterministic validation and portability. They
do not prevent the runtime or other Controlplane subsystems from
implementing those capabilities outside CSL.

## 18. System boundary and runtime responsibilities

CSL defines schema structure and validation semantics. The runtime is
responsible for:

-   Registering resource kinds and immutable schema versions at runtime.
-   Resolving resource kinds and schema versions for validation.
-   Assigning system-managed fields and runtime identity.
-   Applying defaults on creation.
-   Applying partial patches without replacing omitted fields.
-   Distinguishing omission, explicit null, and explicit removal.
-   Validating the complete resulting resource.
-   Persisting updates atomically according to the control plane's
    transaction model.
-   Publishing resource-change events according to that transaction
    model.
-   Enforcing authorization independently of CSL.
-   Protecting sensitive values and redacting them from ordinary reads,
    logs, errors, and event payloads as required by security policy.
-   Managing explicit schema migrations.

The language does not prescribe a storage engine, database, message
broker, API implementation, or controller architecture.

## 19. Examples

### 19.1 VirtualMachine

This example preserves the reusable types, enum values, constraints,
nested objects, defaults, and conditional monitoring from v0.1, while
using v0.2's resource-level fields and lifecycle sections.

``` csl
type domain_name string where matches("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
type machine_model string oneof(q35 i440fx virt pc)
type disk_size bytes range 10GiB to 1TiB

resource VirtualMachine version 1 {
    name: hostname
    namespace: string optional
    site: string length 1 to 255
    address: ip optional
    labels: map[string]string optional
    annotations: map[string]string optional

    desired {
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

        if desired.monitored == true {
            monitoring {
                endpoint: url
                interval: duration = "30s"
                timeout: duration = "5s"
            }
        }
    }

    observed {
        phase: string oneof(pending provisioning running stopped failed)
        ready: bool = false
        ip_address: ip optional
        node: hostname optional
    }
}
```

Example resource document:

``` json
{
  "kind": "VirtualMachine",
  "name": "web-01",
  "site": "production",
  "address": "192.168.10.20",
  "labels": {
    "environment": "production",
    "team": "platform"
  },
  "desired": {
    "cpu": 4,
    "memory": 8589934592,
    "image": "ubuntu-24.04",
    "networks": ["frontend", "backend"],
    "monitored": true,
    "model": "q35",
    "number_of_disks": 2,
    "size_of_disks": [107374182400, 536870912000],
    "backup": {
      "enabled": true,
      "schedule": "0 0 * * *"
    },
    "monitoring": {
      "endpoint": "https://monitoring.example.com",
      "interval": "30s",
      "timeout": "5s"
    }
  },
  "observed": {
    "phase": "running",
    "ready": true,
    "ip_address": "192.168.10.20",
    "node": "node-01"
  }
}
```

The runtime-managed `uid`, `resource_version`, `generation`, timestamps,
and schema version are omitted from this user-authored example for
clarity.

### 19.2 Database

``` csl
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
    customer_id: uuid
    tenant: string
    display_name: string
    labels: map[string]string optional

    desired {
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

        if desired.high_availability == true {
            replication {
                mode: string oneof(sync async)
                replicas: uint8 range 2 to 5
            }
        }
    }

    observed {
        phase: string oneof(pending provisioning running stopped failed)
        endpoint: hostname optional
        port: port
        ready: bool = false
    }
}
```

### 19.3 Network configuration

``` csl
type network_protocol string oneof(tcp udp)

type NetworkPort {
    name: string
    protocol: network_protocol = tcp
    port: port
}

resource NetworkPolicy version 1 {
    name: hostname
    namespace: string optional
    tenant: string
    network_id: uuid

    desired {
        enabled: bool = true
        default_action: string oneof(allow deny) = deny
        source_addresses: cidr[1:64]
        destination_addresses: cidr[1:64]
        ingress: NetworkPort[0:128]
        egress: NetworkPort[0:128]
        allowed_domains: fqdn[0:100] optional
        allowed_sources: cidr[0:256]
        allowed_destinations: cidr[0:256]
        allowed_ports: port[0:128]
    }

    observed {
        phase: string oneof(pending applying active failed)
        last_applied_at: timestamp optional
        error: string optional
    }
}
```

### 19.4 Partial update example

Given a resource with `desired.cpu`, `desired.memory`, and
`desired.image`, this patch changes only the CPU field:

``` text
desired {
    cpu: 8
}
```

The existing memory and image values remain unchanged. The complete
resulting resource is validated before the update is committed.

## 20. Features retained for future consideration

The following items appeared as future considerations or unresolved
proposals in one or both source drafts. Their inclusion here is for
traceability; they are not silently promoted to finalized language
features.

-   Imports and reusable schema packages.
-   Namespaces for custom types.
-   Schema composition and controlled inheritance.
-   More advanced conditional expressions.
-   Cross-field validation blocks.
-   Explicit migration descriptions.
-   Additional built-in types for cloud resources, storage, hardware,
    and Kubernetes-compatible values.
-   Custom validation annotations.
-   Field-level sensitivity and redaction declarations.
-   Read-only and immutable field modifiers.
-   Deprecation annotations.
-   Documentation annotations.
-   Reusable validation constraints.
-   Explicit nullable types or alternative nullable syntax.
-   Further uniqueness rules for collections and map values.
-   More comprehensive date/time and binary serialization rules.
-   Explicit extension fields.
-   Additional pure built-in functions.

These additions should remain declarative and deterministic.

## 21. Open decisions and source-draft conflicts

The following decisions need explicit resolution before CSL v0.3 can be
treated as a stable implementation specification:

1.  **Nullable support:** v0.1 defines `nullable`; v0.2 says explicit
    null is not permitted in that version and defers nullable behavior.
    This draft preserves `nullable` but leaves implementation-profile
    status open.
2.  **Map key types:** v0.1 initially restricts keys to strings; v0.2
    allows scalar keys with stable equality and serialization. This
    draft requires string-key support and leaves additional key types
    open.
3.  **Nested list suffix binding:** the v0.1 prose and example do not
    unambiguously agree about which suffix constrains the outermost
    list. The grammar and examples must be reconciled.
4.  **Conditional expression root:** v0.1 examples use `spec.field`;
    v0.2 uses `desired.field`. This draft adopts `desired.field` and
    `observed.field` for lifecycle sections, and unqualified root fields
    for resource-level fields.
5.  **Conditional declarations and duplicate fields:** the precise
    merge/compatibility behavior for a field declared in multiple
    branches must be formalized.
6.  **Modifier ordering:** the field grammar must specify whether
    constraints and modifiers can appear in any order or have a
    canonical order.
7.  **Regex dialect:** choose and version a deterministic regex dialect
    with complexity limits.
8.  **Duration format:** define accepted units, parsing, normalization,
    and overflow behavior.
9.  **Timestamp normalization:** define whether original offsets are
    preserved or normalized to UTC in canonical storage.
10. **Binary and JSON values:** define canonical wire encodings for
    `binary`, `any`, and `json`.
11. **Map ordering:** define whether map order is semantically
    irrelevant and how canonical serialization orders keys.
12. **Unique semantics:** define equality for each supported element
    type and whether uniqueness can apply beyond lists.
13. **Reserved/contextual keywords:** confirm the contextual-keyword
    model and parser lookahead rules. The v0.2 global reserved-word list
    is not adopted as a restriction on field names.
14. **`any` and `json`:** define whether these types may contain nested
    CSL-specific values or only JSON values.
15. **Sensitive types:** define whether sensitivity is intrinsic to
    `private_key` only or whether general field annotations will be
    introduced.

## 22. Design summary

CSL v0.3 is intended to use a consistent type-oriented syntax:

``` csl
; Primitive
name: string

; Numeric constraint
cpu: int16 range 1 to 128

; Default
enabled: bool = true

; Optional
description: string optional

; Nullable
external_id: string nullable

; Enumeration
model: string oneof(q35 i440fx virt pc)

; Reusable enumerated type
type machine_model string oneof(q35 i440fx virt pc)

; List
networks: string[]

; Bounded list
networks: string[1:8]

; Unique list
networks: string[1:8] unique

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
if desired.enabled == true {
    monitoring {
        endpoint: url
    }
}
```

The guiding principle is: **types define structure, type suffixes define
collection cardinality, constraints define valid values, declarations
define resource structure, and lifecycle sections distinguish desired
intent from observed state.**

CSL should remain small enough to implement consistently across
languages and runtimes, while expressive enough to describe the resource
kinds needed by a dynamic control plane.

------------------------------------------------------------------------

**End of CSL Draft v0.3.**
