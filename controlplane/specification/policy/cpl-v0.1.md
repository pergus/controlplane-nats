# Controlplane Policy Language
## Authorization Proposal — Draft v0.1

**Status:** Proposal  
**Purpose:** Declarative authorization for Controlplane resources and operations

---

## 1. Introduction

Controlplane Policy is a declarative language for defining which identities may perform which operations on resources.

It is separate from Controlplane Schema Language (CSL).

CSL describes resource structure and validation. Policy describes access permissions. The Controlplane runtime evaluates and enforces policy for every API operation.

The policy language should support:

- Role-based access control (RBAC).
- Resource-kind permissions.
- Operation-level permissions.
- Field and section-level permissions.
- Attribute-based access control (ABAC).
- Tenant isolation.
- Service and controller identities.
- Conditional access.
- Partial-update authorization.
- Watch and event-stream authorization.

The initial design favors readable, explicit declarations over a complex general-purpose expression language.

## 2. Core concepts

### 2.1 Subject

A subject is an authenticated identity requesting an operation.

Examples include:

- A human user.
- A service account.
- A controller.
- An automation service.
- An external integration.

A subject has attributes supplied by the authentication system or identity provider.

Example subject attributes:

```text
subject.id
subject.type
subject.tenant
subject.groups
subject.roles
```

The policy engine must not assume that every subject has every attribute.

### 2.2 Resource

A resource is an instance of a registered CSL resource kind.

Policy evaluation may access:

```text
resource.uid
resource.kind
resource.tenant
resource.desired.cpu
resource.observed.phase
```

The available resource fields are determined by its registered CSL schema.

### 2.3 Role

A role is a named collection of permissions.

Roles can be assigned to users, service accounts, or groups outside the resource schema.

### 2.4 Operation

The initial operation set is:

| Operation | Meaning |
|---|---|
| `create` | Create a resource |
| `read` | Read a resource |
| `update` | Modify a resource |
| `delete` | Delete a resource |
| `watch` | Subscribe to resource changes |
| `list` | List resources |

Additional operations may be introduced later, such as `invoke`, `execute`, or `admin`.

## 3. Basic policy syntax

A policy consists of role declarations.

```text
role vm-user {
    resource VirtualMachine {
        read
        create
        update desired
        delete
    }
}
```

This grants the role permission to read, create, and delete VirtualMachine resources, and to update their `desired` section.

The policy does not modify the resource schema.

## 4. Section-level permissions

Permissions can be restricted to particular sections.

```text
role vm-controller {
    resource VirtualMachine {
        read
        update observed
    }
}
```

This grants the controller permission to read VirtualMachine resources and update their observed state.

It does not grant permission to update desired state.

The following is also valid:

```text
role vm-operator {
    resource VirtualMachine {
        read
        update desired
        update observed
    }
}
```

A section-level update permission applies to all fields in that section unless more specific restrictions are declared.

## 5. Field-level permissions

Policies may grant permissions to individual fields.

```text
role vm-operator {
    resource VirtualMachine {
        read
        update desired.cpu
        update desired.memory
    }
}
```

This role may update CPU and memory but cannot modify other desired fields.

Field-level permissions should support nested object fields and map/list paths.

The exact wildcard syntax is reserved for a later revision.

## 6. Attribute-based restrictions

A policy may restrict access based on subject and resource attributes.

```text
role tenant-admin {
    resource VirtualMachine {
        read
        create
        update desired
        delete
    }

    where resource.tenant == subject.tenant
}
```

This role is limited to resources whose tenant matches the subject's tenant.

For create operations, the condition is evaluated against the proposed resource.

For update and delete operations, it is evaluated against the existing resource and, where relevant, the proposed resulting resource.

The runtime must prevent unauthorized changes to attributes used by policy conditions from bypassing the policy.

For example, a tenant-scoped user must not be able to update a resource's tenant field to move it into another tenant.

## 7. Multiple resource kinds

A role can declare permissions for multiple resource kinds.

```text
role platform-operator {
    resource VirtualMachine {
        read
        create
        update desired
        delete
    }

    resource Database {
        read
        create
        update desired
        delete
    }

    resource NetworkPolicy {
        read
        update desired
    }
}
```

Permissions are scoped to the resource kind named in each declaration.

## 8. Controller permissions

Controllers should use dedicated service identities rather than shared user identities.

Example:

```text
role vm-controller {
    resource VirtualMachine {
        read
        update observed
        watch
    }
}
```

A controller should receive only the permissions needed for its responsibilities.

Different controllers may be granted different field-level permissions.

For example:

```text
role vm-network-controller {
    resource VirtualMachine {
        read
        update observed.ip_address
        update observed.node
    }
}
```

The runtime should be able to distinguish controller identities even when multiple controllers operate on the same resource kind.

## 9. List and watch permissions

Listing and watching resources require explicit authorization.

```text
role vm-viewer {
    resource VirtualMachine {
        list
        read
        watch
    }

    where resource.tenant == subject.tenant
}
```

Authorization must apply to every resource returned by a list operation.

Watch authorization must apply to both the initial resource state and subsequent events.

The runtime must not leak unauthorized resources through event notifications, counts, pagination, or error messages.

## 10. Create permissions

Creation is a distinct operation.

```text
role vm-creator {
    resource VirtualMachine {
        create
    }
}
```

A create permission does not automatically grant read, update, or delete permissions.

Policy conditions for creation evaluate the proposed resource, including its resource-level fields and desired state.

The runtime generates the resource `uid`. Clients cannot select or override it.

## 11. Default authorization behavior

The proposed default is **deny by default**.

An operation is allowed only when an applicable policy grants permission.

The policy engine should follow these principles:

1. No matching permission means deny.
2. A matching permission with a failed condition means deny.
3. Explicit denials override grants.
4. Permissions are not implicitly inherited across resource kinds.
5. `create` does not imply `read`, `update`, or `delete`.
6. `read` does not imply `list` or `watch`.
7. `update desired` does not imply `update observed`.
8. Section-level permissions do not imply permissions on unrelated resource-level fields.

The exact composition of multiple roles and policies must be deterministic.

## 12. Partial-update authorization

Partial updates are fundamental to Controlplane.

For example, a request might contain:

```text
desired.cpu = 4
```

The runtime must authorize the specific field being modified.

For a patch containing multiple fields, every requested change must be authorized.

A patch must not be accepted merely because the caller has permission to modify one of its fields.

The runtime should:

1. Authenticate the caller.
2. Identify the requested resource and operation.
3. Load the existing resource, if applicable.
4. Determine the exact set of fields being changed.
5. Evaluate authorization for every changed field.
6. Apply the patch to a candidate resource.
7. Evaluate any conditions affected by the patch.
8. Validate the complete resulting resource using CSL.
9. Commit the update atomically.

Omitted fields are not considered modifications.

Explicit field removal is a modification and requires authorization.

## 13. Policy separation and deployment

Policies should be managed independently of CSL schemas.

A resource kind can be registered without requiring a corresponding policy file, although the default-deny rule means it will not be accessible until suitable permissions are granted.

Policy changes must not require resource schema changes or database migrations.

The runtime should support:

- Registering policies at runtime.
- Updating policies at runtime.
- Removing policies at runtime.
- Versioning policy definitions.
- Auditing policy changes.
- Evaluating policies consistently across API operations.

Policy evaluation should use a consistent policy snapshot for each operation.

## 14. Authentication integration

Authentication is outside the scope of this policy language.

Controlplane should accept authenticated identities from supported authentication mechanisms.

Examples include:

- OIDC/OAuth2.
- Service accounts.
- Mutual TLS identities.
- External identity providers.

The authentication layer establishes who the subject is. The policy engine determines what that subject may do.

The policy engine must not trust arbitrary subject attributes supplied directly by an untrusted API client.

## 15. Security and implementation requirements

The policy engine must:

- Fail closed when policy evaluation fails.
- Avoid leaking information through authorization errors.
- Prevent field-level authorization bypasses.
- Handle missing subject or resource attributes safely.
- Prevent tenant-boundary violations.
- Apply authorization to list and watch operations.
- Apply authorization to both resource state and event delivery.
- Record relevant authorization decisions for auditing.
- Avoid arbitrary code execution in policy expressions.

Policy evaluation should be deterministic and bounded in execution time.

## 16. Complete example

The following example illustrates a basic multi-role setup.

```text
role tenant-user {
    resource VirtualMachine {
        create
        read
        list
        watch
        update desired
        delete
    }

    where resource.tenant == subject.tenant
}

role vm-viewer {
    resource VirtualMachine {
        read
        list
        watch
    }

    where resource.tenant == subject.tenant
}

role vm-controller {
    resource VirtualMachine {
        read
        watch
        update observed
    }
}

role vm-network-controller {
    resource VirtualMachine {
        read
        watch
        update observed.ip_address
        update observed.node
    }
}

role platform-admin {
    resource VirtualMachine {
        create
        read
        list
        watch
        update
        delete
    }

    resource Database {
        create
        read
        list
        watch
        update
        delete
    }

    resource NetworkPolicy {
        create
        read
        list
        watch
        update
        delete
    }
}
```

This is an illustrative policy configuration. Subject-to-role assignments are managed separately.

## 17. Open design questions

The following decisions should be settled before declaring the language stable.

### 17.1 Policy combination

Should permissions from multiple roles combine additively, with explicit deny taking precedence?

Proposed default: yes.

### 17.2 Role inheritance

Should roles be able to inherit other roles?

Proposed initial behavior: no inheritance. Explicitly list permissions to keep evaluation simple and predictable.

### 17.3 Wildcards

Should policies support:

```text
update desired.*
```

Proposed initial behavior: support section-level permissions and exact field paths first. Add wildcards only with clearly defined matching semantics.

### 17.4 Field-level read permissions

Should reading individual fields be independently restricted?

This is useful for secrets, credentials, and sensitive configuration.

Proposed behavior: support field-level read restrictions in a later revision.

### 17.5 Collection operations

Should `list` and `watch` be independent permissions, or should one imply the other?

Proposed behavior: keep them independent.

### 17.6 Policy scope

Should policies apply globally, to a namespace, to a tenant, or to a specific resource kind?

Proposed behavior: support global and resource-kind scopes initially. Additional scopes can be expressed through conditions.

### 17.7 External policy engines

Should Controlplane implement its own policy evaluator, or integrate an existing engine?

This should be evaluated before committing to a complete expression language.

OPA and Cedar are candidates worth investigating. An adapter-based design could allow the runtime to support an external engine without making the policy syntax dependent on a specific implementation.

---

**End of Controlplane Policy v0.1 proposal.**