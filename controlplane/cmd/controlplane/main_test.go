package main

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

func newTestApp(t *testing.T) *App {
	t.Helper()

	natsServer, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	if err != nil {
		t.Fatalf("create test NATS server: %v", err)
	}
	go natsServer.Start()
	if !natsServer.ReadyForConnections(10 * time.Second) {
		t.Fatal("test NATS server did not become ready")
	}
	t.Cleanup(natsServer.Shutdown)

	app, err := connectTo(natsServer.ClientURL())
	if err != nil {
		t.Fatalf("connect to test NATS server: %v", err)
	}
	t.Cleanup(app.close)
	if err := app.ensureInfrastructure(); err != nil {
		t.Fatalf("create test infrastructure: %v", err)
	}

	return app
}

func TestResourceDeleteAndRecreate(t *testing.T) {
	app := newTestApp(t)
	schema := []byte(`#Widget: {
	kind: "Widget"
	metadata: {name: string}
	spec: {value: int}
	generation: int
}`)
	if _, err := app.kindStore.Put("Widget", schema); err != nil {
		t.Fatalf("register test schema: %v", err)
	}
	resource := Resource{
		Kind:     "Widget",
		Metadata: map[string]any{"name": "widget-01"},
		Spec:     map[string]any{"value": 1},
	}

	created, err := app.createResource(resource)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if created.Resource.Generation != 1 {
		t.Fatalf("initial generation = %d, want 1", created.Resource.Generation)
	}
	if _, err := app.createResource(resource); err == nil {
		t.Fatal("create accepted a resource that is already active")
	}

	deleted, err := app.deleteResource("Widget", "widget-01")
	if err != nil {
		t.Fatalf("delete resource: %v", err)
	}
	if deleted.Operation != "deleted" || deleted.Resource.Generation != 2 {
		t.Fatalf("deletion event = operation %q, generation %d; want deleted, 2", deleted.Operation, deleted.Resource.Generation)
	}

	recreated, err := app.createResource(resource)
	if err != nil {
		t.Fatalf("recreate resource: %v", err)
	}
	if recreated.Operation != "created" || recreated.Resource.Generation != 3 {
		t.Fatalf("recreation event = operation %q, generation %d; want created, 3", recreated.Operation, recreated.Resource.Generation)
	}
}

func TestDeleteKindRetainsResourceDeletionPath(t *testing.T) {
	app := newTestApp(t)
	schema := []byte(`#Widget: {
	kind: "Widget"
	metadata: {name: string}
	spec: {value: int}
	generation: int
}`)
	if _, err := app.kindStore.Put("Widget", schema); err != nil {
		t.Fatalf("register test schema: %v", err)
	}
	resource := Resource{
		Kind:     "Widget",
		Metadata: map[string]any{"name": "widget-01"},
		Spec:     map[string]any{"value": 1},
	}
	if _, err := app.createResource(resource); err != nil {
		t.Fatalf("create resource: %v", err)
	}

	if err := app.deleteKind("Widget"); err != nil {
		t.Fatalf("delete kind: %v", err)
	}
	if _, err := app.kindStore.Get("Widget"); err == nil {
		t.Fatal("deleted kind schema is still available")
	}
	if _, err := app.deleteResource("Widget", "widget-01"); err != nil {
		t.Fatalf("delete resource after schema removal: %v", err)
	}
}
