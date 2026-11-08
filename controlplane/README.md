# Controlplane on NATS — proof of concept

This is a small Go proof of concept for a resource-oriented control plane built **on top of an embedded NATS Server with JetStream**. It does not fork or modify NATS.

The POC demonstrates:

- NATS Server embedded directly in the Go process (`serve` and `demo` commands).
- JetStream file persistence.
- Runtime resource-kind registration: CUE schema source is stored in a JetStream KV bucket. Registering a new kind does not require a database migration or server restart.
- Validation of resource writes against the registered CUE schema.
- Partial updates: only supplied JSON fields are merged; all other fields remain unchanged.
- Immutable per-resource event history in a JetStream stream.
- Optimistic concurrency: resource writes use JetStream's expected-last-sequence-per-subject publish precondition.
- Durable event consumption for controllers/observers.

## Requirements

- Go 1.23 or newer.
- No Docker, external NATS process, or SurrealDB required. The `serve` and `demo` commands embed NATS Server.

## Quick start: one-command demonstration

From the project root:

```bash
go mod tidy
go run ./cmd/controlplane demo
```

The demo starts an embedded NATS server on an ephemeral port, creates the JetStream stream and kind registry, registers `VirtualMachine`, creates `vm-01`, applies a partial patch that changes only `spec.cpu`, reads the latest state, and shuts down. Its persistent store is written to the OS temporary directory by default. Override it with `-store ./data`.

## Run a persistent embedded NATS server

Terminal 1:

```bash
go run ./cmd/controlplane serve -store ./data
```

This starts an embedded NATS Server listening on `127.0.0.1:4222` with JetStream enabled. The server persists data under `./data`.

Terminal 2: register the resource kind and create an instance.

```bash
go run ./cmd/controlplane register -file schemas/virtual-machine.cue
go run ./cmd/controlplane create -file examples/vm-01.json
go run ./cmd/controlplane get -kind VirtualMachine -name vm-01
```

Apply a partial update:

```bash
go run ./cmd/controlplane patch -kind VirtualMachine -name vm-01 -file examples/vm-01-patch.json
```

The patch file contains only:

```json
{
  "spec": {
    "cpu": 8
  }
}
```

The resulting resource retains `memory`, `image`, `networks`, and all other attributes.

Delete a resource or remove a kind schema from the registry:

```bash
go run ./cmd/controlplane delete -kind VirtualMachine -name vm-01
go run ./cmd/controlplane delete-kind -kind VirtualMachine
```

Resource deletion appends a tombstone event and preserves the resource history. `get` and `patch` treat the resource as deleted. You can create it again; its generation continues from the prior resource. Kind deletion removes the schema from the active registry but does not delete resource events. You can still read or delete those resources, but you must register the schema again before you can create or patch them.

Watch resource changes from a durable JetStream consumer:

```bash
go run ./cmd/controlplane watch -durable vm-controller
```

The consumer receives historical events on its first run and resumes from its acknowledged position on subsequent runs. Delete the consumer in a real application when it is no longer needed; this POC intentionally keeps it durable.

## Registering another kind

Create a CUE file following the convention of one top-level definition named `#KindName`. For example:

```cue
#Database: {
    kind: "Database"
    metadata: {
        name: string & =~"^[A-Za-z0-9_-]+$"
    }
    spec: {
        engine: "postgres" | "mysql"
        version: string
        storageGB: int & >=1
    }
}
```

Then register it while the server is running:

```bash
go run ./cmd/controlplane register -file schemas/database.cue
```

No Go code changes, schema migration, or server restart are needed. The kind schema is stored in the `KINDS` JetStream KV bucket.

## Configuration

- `NATS_URL` — defaults to `nats://127.0.0.1:4222` for client commands.
- `CONTROLPLANE_STORE` — defaults to `./data` for `serve`.

## Architecture and consistency model

The stream `CONTROLPLANE` stores one full resource snapshot per resource change. Each resource has a subject of the form `cp.resource.<kind>.<name>`. The stream is the durable history and source of truth; the current resource is the last message for that subject. This avoids a dual-write between a resource database and a separate event broker.

A write reads the last message for a resource and publishes the next snapshot with `WithExpectLastSequencePerSubject`. JetStream rejects the publish if another writer has changed that resource since it was read. The caller must retry by re-reading and re-applying its patch. Different resources can be updated independently.

A controller consumes the same stream with a durable consumer and explicit acknowledgements. Delivery is at least once, so controller operations must be idempotent. Events are full snapshots, allowing a controller to recover by reading or replaying state rather than depending on every transient notification being processed exactly once.

## Deliberate limitations

This is an architectural POC, not a production database:

- It has no HTTP API, user authentication, fine-grained authorization, schema migration strategy, or multi-tenant account model yet.
- The CLI accepts JSON instances/patches; CUE is used for runtime kind schemas and validation.
- It supports create, read, partial update, and deletion, but not list queries, indexes, pagination, or garbage collection.
- Resource lookup uses JetStream's last-message-by-subject operation. Listing all resources efficiently would require a materialized index/projection.
- The single embedded server is for local development. Production high availability requires a properly configured multi-node NATS cluster with JetStream replicas and durable storage.
- The demo's `serve` process uses a fixed local address and no authentication. Do not expose it to an untrusted network.
- A single resource's history is stored as full snapshots. A production implementation should define retention, compaction/snapshotting, maximum resource size, and backup/restore policies.
- Schema registration is intentionally simple: one top-level `#Kind` definition per file, and re-registering a kind replaces its current schema. Production needs explicit schema versions and compatibility rules.

## Why this is built above NATS rather than by forking it

The POC treats JetStream as a durable append log and KV as the dynamic schema registry. Resource semantics live in a separate Go service layer. This keeps the project compatible with upstream NATS and lets the resource model evolve without maintaining a NATS Server fork.
