# Viettel Cloud Go SDK

Go SDK for the Viettel Cloud API. The repository is one versioned Go module.
Each service is a subpackage API and `core` holds the shared code; all are
released under the same SemVer tag.

Generated from the OpenAPI specification using
[oapi-codegen v2.8.0](https://github.com/oapi-codegen/oapi-codegen). A facade
generator (`gen_facade.py`) produces ergonomic service wrappers on top of the
generated clients. Shared errors, options, retries, pagination, and transport
behavior are maintained by hand in `core`. The published module depends on
`github.com/google/uuid` only.

## Seed Files

The root module and generation pipeline are configured by a single YAML file.
Only the files below are committed as seeds; the service packages are generated.

```
go/
  Makefile               # Thin wrapper over pipeline/main.py
  pipeline.yaml          # Single source of truth: spec, services, codegen config
  pipeline/              # Orchestrator + spec-normalize + facade scripts
  core/                  # Maintained: errors, retry, pagination, decimal, uuid
  internal/oapi/         # Maintained: runtime surface for generated clients
  server/, blockstorage/, network/   # Generated: client + facade per service
  RELEASING.md           # Release procedure
```

The OpenAPI spec is fetched from the running backend (configured in `pipeline.yaml`),
or supply an existing `openapi.json`. The normalized spec is not checked in;
generated bindings, facades, and documentation are committed for consumer use
without code generation.

## Quick Start

```sh
# One command to go from seed to working SDK
make all
```

```go
import (
    "log/slog"
    "github.com/viettelcloud-oss/sdks/go/blockstorage"
    "github.com/viettelcloud-oss/sdks/go/core"
)

func main() {
    c, err := blockstorage.NewClient("<api-endpoint>",
        blockstorage.WithPAT("<your-token>"),
        blockstorage.WithRetry(core.DefaultRetry()),
        blockstorage.WithUserAgent("myapp/1.0.0"),
        blockstorage.WithLogger(slog.Default()),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Use the client...
    vol, err := c.GetVolume(ctx, volID, blockstorage.GetVolumeParams{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(vol.Name)
}
```

## Installation

```sh
go get github.com/viettelcloud-oss/sdks/go@latest
```

See the repository-level [installation and versioning guide](../README.md#installation)
for release tags and local development with `go work`.

## Usage

### Client Setup

Create a client with `NewClient`. Pass the server URL and functional options:

```go
import (
    "github.com/viettelcloud-oss/sdks/go/server"
    "github.com/viettelcloud-oss/sdks/go/core"
)

c, err := server.NewClient("<api-endpoint>",
    server.WithPAT(token),                              // Authorization: Token <token>
    server.WithRetry(core.DefaultRetry()),           // exponential backoff on 429/5xx
    server.WithUserAgent("myapp/1.0.0"),                // User-Agent header
    server.WithLogger(slog.Default()),                  // debug logging
    server.WithHTTPClient(customHTTPClient),            // custom transport
)
```

#### Available options

| Option | Description |
|--------|-------------|
| `WithPAT(token)` | Set `Authorization: Token <token>` header |
| `WithHTTPClient(c)` | Custom `*http.Client` (connection pooling, proxies, TLS) |
| `WithRetry(cfg)` | Enable retries — `core.DefaultRetry()` or `core.NoRetry()` |
| `WithRetryAllMethods(true)` | Retry POST/PATCH too (default: only GET/PUT/DELETE) |
| `WithUserAgent(ua)` | Prepend the application identifier to the SDK User-Agent |
| `WithLogger(l)` | Enable `slog` debug logging (method, path, status, duration, request-id) |
| `WithRequestEditor(fn)` | Add a custom request editor for advanced use cases |

Public SDK clients accept response bodies up to 16 MiB, including when you use
`WithHTTPClient`. A larger body returns `core.ErrResponseTooLarge`. Use
`errors.Is(err, core.ErrResponseTooLarge)` to identify this error. Use
`errors.As` with `*core.ResponseTooLargeError` to read the HTTP status code and
the request ID. The error never contains the response body.

### Facade Methods

Every OpenAPI operation becomes a typed Go method on `*Client`. The facade
unwraps the HTTP response and returns the parsed payload directly:

| Method shape | Request body | Return | Example |
|---|---|---|---|
| `ListX` | None | `(*PagedXSchema, error)` | `ListVolume(ctx, params)` |
| `GetX` | None | `(*XDetailSchema, error)` | `GetVolume(ctx, id, params)` |
| `CreateX` | `XCreateSchema` | `(*XDetailSchema, error)` | `CreateVolume(ctx, params, body)` |
| `UpdateX` | `XUpdateSchemaPatch` | `(*XDetailSchema, error)` | `UpdateVolume(ctx, id, params, body)` |
| `DeleteX` / action | `XSchema` or none | `error` only | `RetypeVolume(ctx, id, params, body)` |

```go
// Create a volume
var createFrom blockstorage.VolumeCreateSchema_CreateFrom
err := createFrom.FromVolumeCreateEmptySchema(blockstorage.VolumeCreateEmptySchema{
    Size:         100,
    VolumeTypeId: volumeTypeID,
})
if err != nil {
    return err
}
vol, err := c.CreateVolume(ctx, blockstorage.CreateVolumeParams{
    ProjectID: projectID,
}, blockstorage.VolumeCreateSchema{
    Name:       "my-vol",
    CreateFrom: createFrom,
})

// List volumes (single page)
page, err := c.Volumes.Page(ctx, blockstorage.ListVolumeParams{
    ProjectID: projectID,
})
```

`…Params` types carry query, header, and cookie parameters. Required path
parameters (e.g. `volume_id`) are explicit method arguments.

### Pagination

List endpoints support iterator-based pagination. Use `List<Name>Iter`
(Go 1.23+) to fetch results one item at a time, auto-advancing pages:

```go
for item, err := range c.ListVolumeIter(ctx, blockstorage.ListVolumeParams{
    ProjectID: &projectID,
}) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(item.Name)
}
```

Each iterator yields items one at a time and handles page transitions
automatically. Single-page results are available via `List<Name>`.

### Error Handling

API errors return `*APIError` (re-exported from `core`) with structured
fields and sentinel error support:

```go
import (
    "errors"
    "github.com/viettelcloud-oss/sdks/go/blockstorage"
)

vol, err := c.GetVolume(ctx, volID, params)
if err != nil {
    // Quick check with sentinel errors
    if errors.Is(err, blockstorage.ErrNotFound) {
        fmt.Println("volume not found")
    } else if errors.Is(err, blockstorage.ErrValidation) {
        fmt.Println("invalid request")
    }

    // Detailed inspection
    var apiErr *blockstorage.APIError
    if errors.As(err, &apiErr) {
        fmt.Printf("HTTP %d [request_id=%s]\n", apiErr.StatusCode, apiErr.RequestID)
        for _, e := range apiErr.Errors {
            fmt.Printf("  %s: %s (%s)\n", e.Location, e.Message, e.Code)
        }
    }
}
```

#### Sentinel errors

| Sentinel | HTTP status | Use with |
|----------|-------------|----------|
| `ErrNotFound` | 404 | `errors.Is(err, svc.ErrNotFound)` |
| `ErrUnauthorized` | 401, 403 | `errors.Is(err, svc.ErrUnauthorized)` |
| `ErrConflict` | 409 | `errors.Is(err, svc.ErrConflict)` |
| `ErrValidation` | 400, 422 | `errors.Is(err, svc.ErrValidation)` |
| `ErrRateLimited` | 429 | `errors.Is(err, svc.ErrRateLimited)` |
| `ErrInternal` | 5xx | `errors.Is(err, svc.ErrInternal)` |

#### APIError fields

| Field | Type | Description |
|-------|------|-------------|
| `StatusCode` | `int` | HTTP status code |
| `RequestID` | `string` | Server-side request ID (for debugging) |
| `Errors` | `[]ErrorDetail` | Field-level errors (location + code + message) |
| `Raw` | `[]byte` | Raw response body |

#### ErrorDetail fields

| Field | Description |
|-------|-------------|
| `Location` | Field path, e.g. `"flavor_id"` (may be empty) |
| `Code` | Machine-readable code, e.g. `"value_error"` |
| `Message` | Human-readable message |

### Retry & Transport

Retries are built in via `RetryRoundTripper` at the HTTP transport layer.
When enabled, the SDK automatically retries failed requests:

- **429 (Rate Limited):** retries with `Retry-After` header support
- **5xx (Server Error):** retries with exponential backoff + jitter
- **Idempotent by default:** only GET, PUT, DELETE retry; opt-in for POST
  with `WithRetryAllMethods(true)`

```go
c, err := blockstorage.NewClient(url,
    blockstorage.WithPAT(token),
    blockstorage.WithRetry(core.DefaultRetry()),       // 3 attempts, 500ms base, 10s cap
    blockstorage.WithRetryAllMethods(true),               // also retry POST
)
```

Configure custom retry behavior:

```go
blockstorage.WithRetry(&core.RetryConfig{
    MaxAttempts: 5,
    BaseDelay:   200 * time.Millisecond,
    MaxDelay:    30 * time.Second,
})
```

Retries respect context cancellation — if the context is cancelled during
backoff, the retry loop exits immediately. Request bodies are replayed for each
attempt and intermediate response bodies are drained and closed.

The default HTTP client timeout is 30 seconds. Supplying `WithHTTPClient`
preserves the complete client configuration, including its transport, cookie
jar, redirect policy and timeout.

For POST operations that document idempotency-key support:

```go
ctx = blockstorage.WithIdempotencyKey(ctx, "operation-123")
vol, err := c.CreateVolume(ctx, params, body)
```

### Observability

#### User-Agent

Every request includes `viettelcloud-go-sdk/<version>`. `WithUserAgent` prepends the
application identifier:

```go
blockstorage.WithUserAgent("myapp/1.0.0")
```

#### Structured logging

Set via `WithLogger`. Logs at `slog.LevelDebug` with method, path, status,
duration and request-id for each logical request:

```go
blockstorage.WithLogger(slog.Default())
```

### Types

Request and response types are available directly from each service package:

```go
import "github.com/viettelcloud-oss/sdks/go/blockstorage"

vol := &blockstorage.VolumeDetailSchema{Name: "test"}
```

#### Decimals (money, prices, quotas)

Fields the API describes as `number | string` — costs, prices, budgets, quotas —
are typed as `core.Decimal`. It decodes either form and carries the exact
decimal text, so no precision is lost the way it would be through `float64`:

```go
cost.HourlyPrice.String()   // "0.0416666667" — exact, for display
cost.HourlyPrice.Float64()  // 0.0416666667, error — lossy, for arithmetic you
                            // do not mind rounding
cost.HourlyPrice.Rat()      // *big.Rat, error — exact, for arithmetic you do
cost.HourlyPrice.Cmp(other) // int, error — numeric comparison

amount := core.MustDecimal("19.99")   // panics on an invalid literal
amount, err := core.NewDecimal(input) // returns an error instead
```

#### Resource IDs

All resource identifiers are `core.UUID` — an alias for
`github.com/google/uuid.UUID`. Existing code using that package keeps working:

```go
id := core.MustParseUUID("3fa85f64-5717-4562-b3fc-2c963f66afa6") // panics if invalid
id, err := core.ParseUUID(input)                                 // returns error
id := core.NewUUID()                                             // random v4
if id == core.NilUUID { /* unset */ }
```

## Versioning and releases

The entire module uses one SemVer version. Since this module is located in the
repository's `go/` subdirectory, release tags have the form `go/vX.Y.Z`.
Generated `core.Version` records the SDK version and `core.SpecVersion` records
the upstream OpenAPI `info.version`. See [RELEASING.md](RELEASING.md).

## Development

### Prerequisites

- Go 1.26.8 or later
- Python 3 (for pipeline orchestration, normalization, facade generation)
- PyYAML: `pip install pyyaml`
- `curl` (optional; only needed to fetch spec from backend)
- Network access to your Go module proxy (for initial `make all`)

### Make Targets

| Command | Description |
|---------|-------------|
| `make help` | Show all available targets |
| `make init` | Create `go.mod`, or sync its module path and Go version to `pipeline.yaml` |
| `make all` | Full pipeline: spec + generate, then tidy/build/vet |
| `make spec` | Normalize the spec, reusing the existing `openapi.json` |
| `make spec FETCH_SPEC=1` | Fetch the spec from the backend first, then normalize |
| `make generate` | Re-normalize each local raw spec, then generate clients + facades for all services |
| `make generate SERVICES="server network"` | Generate only the named services |
| `make build` | Compile every package in the module |
| `make vet` | Run `go vet` on every package in the module |
| `make tidy` | Run `go mod tidy` for the module |
| `make clean` | Remove generated code and facades |
| `make clean-all` | Also remove the fetched `openapi.json` |

### Checks

| Command | Description |
|---------|-------------|
| `make verify` | `fmt-check`, `fix-check`, `vet`, `lint`, `test`, `build` — exactly what CI runs on a merge request |
| `make tools` | Install the pinned `golangci-lint`, `gofumpt` and `goimports` from `../tools` |
| `make fmt` | gofmt + gofumpt + goimports over the hand-maintained sources |
| `make fmt-check` | Fail if `make fmt` would rewrite anything |
| `make fix` | Rewrite `core/` and `internal/` using deprecated APIs (`go fix`) |
| `make fix-check` | Fail if `make fix` would rewrite anything |
| `make lint` | `golangci-lint run ./...` |
| `make test` | `go test -race -cover ./...` |
| `make spec-version` | Print the upstream `info.version` next to `core.SpecVersion` |

Run `make verify` before pushing — it is the same command
[`.jenkins/check_merge_request.yml`](../.jenkins/check_merge_request.yml) runs,
so a green local run means a green pipeline.

### Continuous integration

| Pipeline | Trigger | What it does |
|---|---|---|
| `check_merge_request` | push to an open MR, or a `recheck` comment | commit-title format, then `make verify` |
| `check_main` | push to `main` | `make verify` on the merge commit |
| `release` | manual | semantic-release; see [RELEASING.md](RELEASING.md) |

Both check pipelines run in `cmp/golang-terraform`, the image the terraform
provider already builds — the Go toolchain is all they need.

Nothing is deployed from this repository, so there is no deploy pipeline. What
takes its place is a check that the committed bindings still match the backend
they were generated from; that is tracked separately from this CI setup.

### Pipeline

```
pipeline.yaml
  │
  ├─ Spec fetch + normalize
  │  • Fetch from backend (or reuse openapi.json)
  │  • Strip Django operationId prefixes
  │  • Rewrite Pydantic shapes (unions, decimals, uuids, etc)
  │  → openapi_edited.json
  │
  └─ Generate (per service)
     • oapi-codegen v2.8.0 → <service>/internal/gen/
     • Repoint runtime import to internal/oapi
     • gen_facade.py → <service>/client.go, operations.go, …
```

All configuration — spec URL, services, oapi-codegen options, normalize rules —
lives in `pipeline.yaml`. The Makefile delegates to `python3 pipeline/main.py`,
which reads the config and orchestrates the steps.

### Add a New Service

1. Add the service to `pipeline.yaml` under `services:`:
   ```yaml
   - name: loadbalancer
     folder: loadbalancer
     tag: "Load Balancing"
   ```

2. Generate:
   ```sh
   make generate && make tidy && make build
   ```

   Or generate only the new service:
   ```sh
   make generate SERVICES="loadbalancer" && make tidy && make build
   ```

### Regenerate After Spec Update

```sh
# Fetch from configured backend and regenerate everything
make all

# Fetch from a different backend (override pipeline.yaml):
make all SPEC_URL=https://staging-api.example.com/v2/openapi.json

# Regenerate specific services only (reuse existing spec):
make generate SERVICES="server blockstorage"
```
