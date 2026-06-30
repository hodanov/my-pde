# ai-bridge Coding Conventions

## Table Driven Tests

MUST use Table Driven Test format. Define test cases as a struct slice with `name` field.
ALWAYS add `t.Parallel()` to both the test function and each subtest.

```go
func TestParseRequest(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name       string
        content    string // input
        wantPrompt string // expected output
        wantErr    bool   // expected error
    }{
        {name: "valid request", content: `{"prompt":"hello"}`, wantPrompt: "hello"},
        {name: "missing prompt", content: `{}`, wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            // ...
        })
    }
}
```

Struct field naming: `name` for case name, descriptive names for input, `want*` for expected output, `wantErr bool` for error presence.

Exception: if test cases need fundamentally different verification logic, use separate test functions.

### Exception: `t.Setenv`

`t.Setenv` panics in parallel tests (Go 1.22+). MUST NOT add `t.Parallel()` to any test function or subtest that uses `t.Setenv`.

## Error Variable Naming

MUST NOT reuse or shadow `err`. ALWAYS give each error variable a unique descriptive name: `<verb><Target>Err`.

**Bad:**

```go
data, err := os.ReadFile(path)
if err != nil { ... }
result, err := json.Unmarshal(data, &v) // reuses err
```

**Good:**

```go
data, readErr := os.ReadFile(path)
if readErr != nil { ... }
unmarshalErr := json.Unmarshal(data, &v)
if unmarshalErr != nil { ... }
```

This applies to `if` scopes too — `if chmodErr := f.Chmod(0o755); chmodErr != nil { ... }`.

## Package Scope

MUST keep functions/methods unexported (lowercase) unless called from another package or required by an interface. Same-package test files (`_test.go`) do not count as "another package."

## Architecture & Mock Generation

The daemon follows DDD + clean architecture: `cmd → infra → usecase → domain` (dependency points inward only). Layer-crossing side effects MUST go through a port (interface) declared in the `internal/usecase/port` package (`port.go`); the implementation lives in `internal/infra/`. Referencing ports via the `port.*` qualifier (e.g. a `port.RequestRepository` field) makes boundary-crossing calls explicit at the call site. Keep `internal/domain/` pure (no filesystem, env, process, clock, or third-party I/O).

Mocks for the ports are generated with `go.uber.org/mock` (mockgen), pinned as a `go tool` in `go.mod`. After adding or changing a port, regenerate with `make generate` (`go generate ./...`) and commit the result. Use the generated mocks (`internal/usecase/port/mock`) to unit-test use cases; do not hand-write test doubles for ports.
