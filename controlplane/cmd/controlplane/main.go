// Package main provides a command-line control plane that validates resources
// with CUE and stores resource events in NATS JetStream.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

const (
	// streamName stores resource events.
	streamName = "CONTROLPLANE"
	// kindsBucket stores registered CUE schemas.
	kindsBucket = "KINDS"
	// resourcePrefix is the subject prefix for resource events.
	resourcePrefix = "cp.resource"
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Resource is a typed resource document stored by the control plane.
type Resource struct {
	Kind       string         `json:"kind"`
	Metadata   map[string]any `json:"metadata"`
	Spec       map[string]any `json:"spec"`
	Generation uint64         `json:"generation"`
}

// ResourceEvent records a resource change in the JetStream stream.
type ResourceEvent struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Revision  uint64    `json:"revision,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Resource  Resource  `json:"resource"`
}

// App holds the NATS connections and stores used by a command.
type App struct {
	connection *nats.Conn
	jetStream  nats.JetStreamContext
	kindStore  nats.KeyValue
}

// main dispatches the requested command and reports any error.
func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error

	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "register":
		err = register(os.Args[2:])
	case "create":
		err = create(os.Args[2:])
	case "patch":
		err = patch(os.Args[2:])
	case "get":
		err = get(os.Args[2:])
	case "delete":
		err = deleteResourceCommand(os.Args[2:])
	case "delete-kind":
		err = deleteKindCommand(os.Args[2:])
	case "watch":
		err = watch(os.Args[2:])
	case "demo":
		err = demo(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// usage prints the available commands and environment variables.
func usage() {
	fmt.Print(`Controlplane-on-NATS proof of concept

Commands:
  serve       Run an embedded NATS Server with JetStream
  register    Register a CUE resource-kind schema
  create      Create a resource from JSON
  patch       Apply a partial JSON patch
  get         Read the latest resource state
  delete      Delete a resource
  delete-kind Delete a resource kind schema
  watch       Consume resource events (durable JetStream consumer)
  demo        Run an end-to-end demonstration in one process

Environment:
  NATS_URL=nats://127.0.0.1:4222
  CONTROLPLANE_STORE=./data
`)
}

// serve starts an embedded NATS server and waits for a termination signal.
func serve(args []string) error {
	flagSet := flag.NewFlagSet("serve", flag.ContinueOnError)
	storeDir := flagSet.String("store", env("CONTROLPLANE_STORE", "./data"), "JetStream file store directory")

	port := flagSet.Int("port", 4222, "NATS client port")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if err := os.MkdirAll(*storeDir, 0o755); err != nil {
		return err
	}

	natsServer, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: *port, JetStream: true, StoreDir: *storeDir, NoLog: false, NoSigs: true})
	if err != nil {
		return err
	}

	go natsServer.Start()

	if !natsServer.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("embedded NATS server did not become ready")
	}
	defer natsServer.Shutdown()

	app, err := connectTo(fmt.Sprintf("nats://127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	defer app.close()

	if err := app.ensureInfrastructure(); err != nil {
		return err
	}

	fmt.Printf("Embedded NATS + JetStream ready at nats://127.0.0.1:%d\n", *port)
	fmt.Printf("Persistent JetStream directory: %s\n", *storeDir)

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	<-signalContext.Done()

	return nil
}

// connect opens a NATS connection using NATS_URL or the default URL.
func connect() (*App, error) { return connectTo(env("NATS_URL", nats.DefaultURL)) }

// connectTo opens a NATS connection and initializes its JetStream context.
func connectTo(url string) (*App, error) {
	connection, err := nats.Connect(url, nats.Name("controlplane-poc"), nats.Timeout(5*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return nil, err
	}

	jetStream, err := connection.JetStream()
	if err != nil {
		connection.Close()
		return nil, err
	}

	return &App{connection: connection, jetStream: jetStream}, nil
}

// close releases the app's NATS connection.
func (app *App) close() { app.connection.Close() }

// ensureInfrastructure creates the event stream and schema key-value bucket if needed.
func (app *App) ensureInfrastructure() error {
	if _, err := app.jetStream.StreamInfo(streamName); err != nil {
		_, err = app.jetStream.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{resourcePrefix + ".>"}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, Discard: nats.DiscardOld, Replicas: 1, AllowDirect: true})
		if err != nil {
			return fmt.Errorf("create stream: %w", err)
		}
	}

	var err error

	app.kindStore, err = app.jetStream.KeyValue(kindsBucket)
	if err == nil {
		return nil
	}

	app.kindStore, err = app.jetStream.CreateKeyValue(&nats.KeyValueConfig{Bucket: kindsBucket, Description: "Runtime resource kind definitions", Storage: nats.FileStorage, History: 10, Replicas: 1})
	if err != nil {
		return fmt.Errorf("create kind registry: %w", err)
	}

	return nil
}
// register validates and stores a CUE schema in the kind registry.
func register(args []string) error {
	flagSet := flag.NewFlagSet("register", flag.ContinueOnError)

	file := flagSet.String("file", "", "CUE schema file")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if *file == "" {
		return fmt.Errorf("-file is required")
	}

	source, err := os.ReadFile(*file)
	if err != nil {
		return err
	}

	kind, err := kindFromSchema(source)
	if err != nil {
		return err
	}

	app, err := connect()
	if err != nil {
		return err
	}
	defer app.close()

	if err := app.ensureInfrastructure(); err != nil {
		return err
	}

	_, err = app.kindStore.Put(kind, source)
	if err != nil {
		return err
	}

	fmt.Printf("Registered kind %q (schema stored in JetStream KV)\n", kind)

	return nil
}

// create reads, validates, and stores a resource from a JSON file.
func create(args []string) error {
	flagSet := flag.NewFlagSet("create", flag.ContinueOnError)

	file := flagSet.String("file", "", "JSON resource file")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if *file == "" {
		return fmt.Errorf("-file is required")
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	// UseNumber preserves integer values until they are normalized for CUE.
	decoder.UseNumber()

	var resource Resource
	if err := decoder.Decode(&resource); err != nil {
		return fmt.Errorf("decode resource JSON: %w", err)
	}

	if err := normalizeResourceNumbers(&resource); err != nil {
		return fmt.Errorf("normalize resource numbers: %w", err)
	}

	app, err := connect()
	if err != nil {
		return err
	}
	defer app.close()

	if err := app.ensureInfrastructure(); err != nil {
		return err
	}

	event, err := app.createResource(resource)
	if err != nil {
		return err
	}

	return printJSON(event)
}

// patch deep-merges a JSON patch into the latest resource and appends an event.
func patch(args []string) error {
	flagSet := flag.NewFlagSet("patch", flag.ContinueOnError)
	kind := flagSet.String("kind", "", "Resource kind")
	name := flagSet.String("name", "", "Resource name")

	file := flagSet.String("file", "", "Partial JSON patch")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if *kind == "" || *name == "" || *file == "" {
		return fmt.Errorf("-kind, -name and -file are required")
	}

	if err := validateToken(*kind); err != nil {
		return err
	}

	if err := validateToken(*name); err != nil {
		return err
	}

	patchData, err := os.ReadFile(*file)
	if err != nil {
		return err
	}

	patchDecoder := json.NewDecoder(bytes.NewReader(patchData))
	patchDecoder.UseNumber()

	var changes map[string]any
	if err := patchDecoder.Decode(&changes); err != nil {
		return err
	}

	app, err := connect()
	if err != nil {
		return err
	}
	defer app.close()

	if err := app.ensureInfrastructure(); err != nil {
		return err
	}

	current, sequence, err := app.latest(*kind, *name)
	if err != nil {
		return err
	}
	if current.Operation == "deleted" {
		return fmt.Errorf("resource %s/%s has been deleted", *kind, *name)
	}

	merged := map[string]any{}

	encoded, _ := json.Marshal(current.Resource)
	if err := json.Unmarshal(encoded, &merged); err != nil {
		return err
	}

	deepMerge(merged, changes)

	var updated Resource

	mergedResourceJSON, err := json.Marshal(merged)
	if err != nil {
		return err
	}

	updatedDecoder := json.NewDecoder(bytes.NewReader(mergedResourceJSON))
	updatedDecoder.UseNumber()

	if err := updatedDecoder.Decode(&updated); err != nil {
		return err
	}

	if err := normalizeResourceNumbers(&updated); err != nil {
		return fmt.Errorf("normalize patched resource numbers: %w", err)
	}

	if updated.Kind != *kind || updated.Metadata["name"] != *name {
		return fmt.Errorf("kind and metadata.name are immutable")
	}

	updated.Generation = current.Resource.Generation + 1

	event, err := app.writeResource(updated, "updated", sequence)
	if err != nil {
		return err
	}

	return printJSON(event)
}

// get prints the latest stored state for a resource.
func get(args []string) error {
	flagSet := flag.NewFlagSet("get", flag.ContinueOnError)
	kind := flagSet.String("kind", "", "Resource kind")

	name := flagSet.String("name", "", "Resource name")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if *kind == "" || *name == "" {
		return fmt.Errorf("-kind and -name are required")
	}

	app, err := connect()
	if err != nil {
		return err
	}
	defer app.close()

	if err := app.ensureInfrastructure(); err != nil {
		return err
	}

	event, _, err := app.latest(*kind, *name)
	if err != nil {
		return err
	}
	if event.Operation == "deleted" {
		return fmt.Errorf("resource %s/%s has been deleted", *kind, *name)
	}

	return printJSON(event.Resource)
}

// deleteResourceCommand appends a deletion event for one resource.
func deleteResourceCommand(args []string) error {
	flagSet := flag.NewFlagSet("delete", flag.ContinueOnError)
	kind := flagSet.String("kind", "", "Resource kind")
	name := flagSet.String("name", "", "Resource name")
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	if *kind == "" || *name == "" {
		return fmt.Errorf("-kind and -name are required")
	}
	app, err := connect()
	if err != nil {
		return err
	}
	defer app.close()
	if err := app.ensureInfrastructure(); err != nil {
		return err
	}
	event, err := app.deleteResource(*kind, *name)
	if err != nil {
		return err
	}
	return printJSON(event)
}

// deleteKindCommand removes a resource kind schema from the registry.
func deleteKindCommand(args []string) error {
	flagSet := flag.NewFlagSet("delete-kind", flag.ContinueOnError)
	kind := flagSet.String("kind", "", "Resource kind")
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	if *kind == "" {
		return fmt.Errorf("-kind is required")
	}
	app, err := connect()
	if err != nil {
		return err
	}
	defer app.close()
	if err := app.ensureInfrastructure(); err != nil {
		return err
	}
	if err := app.deleteKind(*kind); err != nil {
		return err
	}
	fmt.Printf("Deleted resource kind %q from the schema registry; resource history was retained\n", *kind)
	return nil
}

// watch consumes resource events with a durable JetStream subscription.
func watch(args []string) error {
	flagSet := flag.NewFlagSet("watch", flag.ContinueOnError)

	durable := flagSet.String("durable", "controlplane-observer", "Durable consumer name")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	app, err := connect()
	if err != nil {
		return err
	}
	defer app.close()

	if err := app.ensureInfrastructure(); err != nil {
		return err
	}

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	subscription, err := app.jetStream.Subscribe(resourcePrefix+".>", func(message *nats.Msg) {
		var event ResourceEvent
		if err := json.Unmarshal(message.Data, &event); err != nil {
			fmt.Fprintln(os.Stderr, "invalid event:", err)

			_ = message.Term()

			return
		}

		if metadata, err := message.Metadata(); err == nil {
			event.Revision = metadata.Sequence.Stream
		}

		fmt.Printf("event=%s operation=%s resource=%s/%v revision=%d\n", event.ID, event.Operation, event.Resource.Kind, event.Resource.Metadata["name"], event.Revision)

		if err := message.Ack(); err != nil {
			fmt.Fprintln(os.Stderr, "ack:", err)
		}
	}, nats.Durable(*durable), nats.DeliverAll(), nats.ManualAck(), nats.AckWait(30*time.Second))
	if err != nil {
		return err
	}
	defer subscription.Unsubscribe()

	fmt.Printf("Watching %s (durable=%s); Ctrl-C to stop\n", resourcePrefix+".>", *durable)
	<-signalContext.Done()

	return nil
}

// writeResource validates a resource and publishes an event with a sequence check.
func (app *App) writeResource(resource Resource, operation string, expected uint64) (ResourceEvent, error) {
	name, ok := resource.Metadata["name"].(string)
	if !ok || name == "" {
		return ResourceEvent{}, fmt.Errorf("metadata.name must be a non-empty string")
	}

	if err := validateToken(resource.Kind); err != nil {
		return ResourceEvent{}, err
	}

	if err := validateToken(name); err != nil {
		return ResourceEvent{}, err
	}

	if resource.Spec == nil {
		return ResourceEvent{}, fmt.Errorf("spec is required")
	}

	if operation == "created" && resource.Generation == 0 {
		resource.Generation = 1
	}

	if operation != "deleted" {
		if err := app.validate(resource); err != nil {
			return ResourceEvent{}, err
		}
	}

	subject := resourceSubject(resource.Kind, name)
	id := fmt.Sprintf("%s-%d", subject, time.Now().UnixNano())
	event := ResourceEvent{ID: id, Operation: operation, Timestamp: time.Now().UTC(), Resource: resource}

	payload, err := json.Marshal(event)
	if err != nil {
		return ResourceEvent{}, err
	}
	// The expected sequence prevents a stale patch from overwriting a newer event.
	publishAck, err := app.jetStream.Publish(subject, payload, nats.MsgId(id), nats.ExpectLastSequencePerSubject(expected))
	if err != nil {
		return ResourceEvent{}, fmt.Errorf("resource write rejected (possible concurrent update): %w", err)
	}

	event.Revision = publishAck.Sequence

	return event, nil
}

// createResource rejects live duplicates and permits recreation after a tombstone.
func (app *App) createResource(resource Resource) (ResourceEvent, error) {
	name, ok := resource.Metadata["name"].(string)
	if !ok || name == "" {
		return ResourceEvent{}, fmt.Errorf("metadata.name must be a non-empty string")
	}

	expectedSequence := uint64(0)
	previousEvent, previousSequence, err := app.latest(resource.Kind, name)
	if err == nil {
		if previousEvent.Operation != "deleted" {
			return ResourceEvent{}, fmt.Errorf("resource %s/%s already exists", resource.Kind, name)
		}
		expectedSequence = previousSequence
		resource.Generation = previousEvent.Resource.Generation + 1
	} else if !errors.Is(err, nats.ErrMsgNotFound) {
		return ResourceEvent{}, err
	}

	return app.writeResource(resource, "created", expectedSequence)
}

// deleteResource appends a tombstone while preserving the previous resource snapshot.
func (app *App) deleteResource(kind, name string) (ResourceEvent, error) {
	currentEvent, sequence, err := app.latest(kind, name)
	if err != nil {
		return ResourceEvent{}, err
	}
	if currentEvent.Operation == "deleted" {
		return ResourceEvent{}, fmt.Errorf("resource %s/%s has already been deleted", kind, name)
	}

	resource := currentEvent.Resource
	resource.Generation++
	return app.writeResource(resource, "deleted", sequence)
}

// deleteKind removes a kind's active schema but retains its KV history.
func (app *App) deleteKind(kind string) error {
	if err := validateToken(kind); err != nil {
		return err
	}
	if _, err := app.kindStore.Get(kind); err != nil {
		return fmt.Errorf("resource kind %q is not registered: %w", kind, err)
	}
	if err := app.kindStore.Delete(kind); err != nil {
		return fmt.Errorf("delete resource kind %q: %w", kind, err)
	}
	return nil
}

// validate checks a resource against the CUE schema registered for its kind.
func (app *App) validate(resource Resource) error {
	schemaEntry, err := app.kindStore.Get(resource.Kind)
	if err != nil {
		return fmt.Errorf("kind %q is not registered: %w", resource.Kind, err)
	}

	cueContext := cuecontext.New()

	schema := cueContext.CompileString(string(schemaEntry.Value()))
	if schema.Err() != nil {
		return schema.Err()
	}

	path := cue.ParsePath("#" + resource.Kind)

	definition := schema.LookupPath(path)
	if !definition.Exists() {
		return fmt.Errorf("schema file must define #%s", resource.Kind)
	}

	value := cueContext.Encode(resource)
	if err := value.Err(); err != nil {
		return err
	}

	if err := definition.Unify(value).Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	return nil
}

// latest returns the last event and stream sequence for a resource subject.
func (app *App) latest(kind, name string) (ResourceEvent, uint64, error) {
	if err := validateToken(kind); err != nil {
		return ResourceEvent{}, 0, err
	}

	if err := validateToken(name); err != nil {
		return ResourceEvent{}, 0, err
	}

	message, err := app.jetStream.GetLastMsg(streamName, resourceSubject(kind, name))
	if err != nil {
		return ResourceEvent{}, 0, fmt.Errorf("resource %s/%s not found: %w", kind, name, err)
	}

	var event ResourceEvent
	if err := json.Unmarshal(message.Data, &event); err != nil {
		return ResourceEvent{}, 0, err
	}

	event.Revision = message.Sequence

	return event, message.Sequence, nil
}

// kindFromSchema validates CUE syntax and extracts the top-level #Kind name.
func kindFromSchema(source []byte) (string, error) {
	if _, err := parser.ParseFile("schema.cue", source); err != nil {
		return "", err
	}

	cueContext := cuecontext.New()

	compiledSchema := cueContext.CompileBytes(source)
	if compiledSchema.Err() != nil {
		return "", compiledSchema.Err()
	}
	// The POC convention is one top-level definition named #Kind.
	text := string(source)

	matches := regexp.MustCompile(`(?m)^#([A-Za-z][A-Za-z0-9_-]*):`).FindStringSubmatch(text)
	if len(matches) != 2 {
		return "", fmt.Errorf("schema must define one top-level #Kind definition")
	}

	return matches[1], nil
}

// deepMerge merges nested objects and replaces scalar or array values.
func deepMerge(destination, patchValues map[string]any) {
	for fieldName, patchValue := range patchValues {
		if nestedPatch, ok := patchValue.(map[string]any); ok {
			if nestedDestination, ok := destination[fieldName].(map[string]any); ok {
				deepMerge(nestedDestination, nestedPatch)
			} else {
				nestedDestination := map[string]any{}
				deepMerge(nestedDestination, nestedPatch)
				destination[fieldName] = nestedDestination
			}
		} else {
			destination[fieldName] = patchValue
		}
	}
}

// resourceSubject builds the JetStream subject for one kind and resource name.
func resourceSubject(kind, name string) string { return resourcePrefix + "." + kind + "." + name }

// validateToken checks whether a value is safe to use in a NATS subject token.
func validateToken(value string) error {
	if !tokenPattern.MatchString(value) {
		return fmt.Errorf("%q must contain only letters, digits, underscore or hyphen", value)
	}

	return nil
}

// normalizeJSONNumbers converts json.Number values to Go integers or floats recursively.
func normalizeJSONNumbers(value any) (any, error) {
	switch typedValue := value.(type) {
	case json.Number:
		// Prefer integers so CUE integer constraints do not receive float values.
		if integerValue, err := typedValue.Int64(); err == nil {
			return integerValue, nil
		}

		if floatValue, err := typedValue.Float64(); err == nil {
			return floatValue, nil
		}

		return nil, fmt.Errorf("invalid JSON number %q", typedValue)

	case map[string]any:
		result := make(map[string]any, len(typedValue))
		for fieldName, nestedValue := range typedValue {
			normalized, err := normalizeJSONNumbers(nestedValue)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fieldName, err)
			}

			result[fieldName] = normalized
		}

		return result, nil

	case []any:
		result := make([]any, len(typedValue))
		for index, nestedValue := range typedValue {
			normalized, err := normalizeJSONNumbers(nestedValue)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", index, err)
			}

			result[index] = normalized
		}

		return result, nil

	default:
		return value, nil
	}
}

// normalizeResourceNumbers converts JSON numbers in resource metadata and spec maps.
func normalizeResourceNumbers(resource *Resource) error {
	if resource.Metadata != nil {
		normalized, err := normalizeJSONNumbers(resource.Metadata)
		if err != nil {
			return fmt.Errorf("metadata: %w", err)
		}

		resource.Metadata = normalized.(map[string]any)
	}

	if resource.Spec != nil {
		normalized, err := normalizeJSONNumbers(resource.Spec)
		if err != nil {
			return fmt.Errorf("spec: %w", err)
		}

		resource.Spec = normalized.(map[string]any)
	}

	return nil
}

// printJSON writes an indented JSON value to standard output.
func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	return enc.Encode(value)
}

// env returns an environment variable or its fallback when it is empty.
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

// demo runs a create, patch, and read sequence against an embedded NATS server.
func demo(args []string) error {
	flagSet := flag.NewFlagSet("demo", flag.ContinueOnError)

	storeDir := flagSet.String("store", filepath.Join(os.TempDir(), "controlplane-nats-poc"), "temporary embedded JetStream store")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if err := os.MkdirAll(*storeDir, 0o755); err != nil {
		return err
	}

	natsServer, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: *storeDir, NoLog: true, NoSigs: true})
	if err != nil {
		return err
	}

	go natsServer.Start()

	if !natsServer.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("embedded NATS server did not become ready")
	}
	defer natsServer.Shutdown()

	url := natsServer.ClientURL()

	app, err := connectTo(url)
	if err != nil {
		return err
	}
	defer app.close()

	if err := app.ensureInfrastructure(); err != nil {
		return err
	}

	schema, err := os.ReadFile("schemas/virtual-machine.cue")
	if err != nil {
		schema, err = os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "schemas", "virtual-machine.cue"))
	}

	if err != nil {
		return fmt.Errorf("run demo from the project root so schemas/virtual-machine.cue is available: %w", err)
	}

	kind, err := kindFromSchema(schema)
	if err != nil {
		return err
	}

	if _, err := app.kindStore.Put(kind, schema); err != nil {
		return err
	}

	resourceData, err := os.ReadFile("examples/vm-01.json")
	if err != nil {
		return err
	}

	var resource Resource
	if err := json.Unmarshal(resourceData, &resource); err != nil {
		return err
	}

	created, err := app.writeResource(resource, "created", 0)
	if err != nil {
		return err
	}

	fmt.Println("CREATED:")

	if err := printJSON(created); err != nil {
		return err
	}

	patchData, err := os.ReadFile("examples/vm-01-patch.json")
	if err != nil {
		return err
	}

	var changes map[string]any
	if err := json.Unmarshal(patchData, &changes); err != nil {
		return err
	}

	current, sequence, err := app.latest("VirtualMachine", "vm-01")
	if err != nil {
		return err
	}

	merged := map[string]any{}
	resourceJSON, _ := json.Marshal(current.Resource)
	_ = json.Unmarshal(resourceJSON, &merged)
	deepMerge(merged, changes)
	resourceJSON, _ = json.Marshal(merged)

	var updated Resource

	_ = json.Unmarshal(resourceJSON, &updated)
	updated.Generation = current.Resource.Generation + 1

	patched, err := app.writeResource(updated, "updated", sequence)
	if err != nil {
		return err
	}

	fmt.Println("PATCHED (only spec.cpu changed; other fields were preserved):")

	if err := printJSON(patched); err != nil {
		return err
	}

	latest, _, err := app.latest("VirtualMachine", "vm-01")
	if err != nil {
		return err
	}

	fmt.Println("READ BACK:")

	return printJSON(latest.Resource)
}
