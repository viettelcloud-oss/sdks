# Viettel Cloud Go SDK

Go SDK for the Viettel Cloud API. The repository is one versioned Go module.
Each service is a subpackage API and `core` holds the shared code; all are
released under the same SemVer tag.

Generated from the OpenAPI specification using
[oapi-codegen v2.8.0](https://github.com/oapi-codegen/oapi-codegen). A facade
generator (`gen_facade.py`) produces ergonomic service wrappers on top of the
generated clients. Shared errors, options, retries, pagination, and transport
behavior are maintained by hand in `core`. The published module has two
direct dependencies: `github.com/google/uuid` and
`github.com/oapi-codegen/nullable`.

## Seed Files

The root module and generation pipeline are configured by a single YAML file.
Only the files below are committed as seeds; the service packages are generated.

```
go/
  Makefile               # Thin wrapper over pipeline/main.py
  pipeline.yaml          # Single source of truth: spec, services, codegen config
  pipeline/              # Orchestrator + spec-normalize + facade scripts
  core/                  # Maintained: errors, retry, pagination, decimal, uuid, nullable
  internal/oapi/         # Maintained: runtime surface for generated clients
  <service>/             # Generated: client + facade per service (see pipeline.yaml)
  <service>/doc.go       # Maintained, where present: package documentation
  <service>/README.md    # Maintained, where present: service quick start
  RELEASING.md           # Release procedure
```

Each service reads its own OpenAPI spec. The pipeline downloads it from the
backend to `openapi.<service>.json`. The backend address comes from
`SPEC_BASE_URL` in `go/.env`, and the path of each spec comes from
`pipeline.yaml`. You can also put the raw spec files in
place by hand. Raw and normalized specs are not committed. The generated
bindings and facades are committed, so consumers do not need code generation.

## Quick Start

Copy `.env.example` to `.env` and set `SPEC_BASE_URL` to the backend address.
Git ignores `.env`, so do not commit it:

```sh
cp .env.example .env
```

```sh
# One command to go from seed to working SDK. The raw specs are not
# committed, so a fresh checkout must fetch them.
make all FETCH_SPEC=1
```

```go
import (
    "context"
    "fmt"
    "log"
    "log/slog"

    "github.com/viettelcloud-oss/sdks/go/blockstorage"
    "github.com/viettelcloud-oss/sdks/go/core"
)

func main() {
    ctx := context.Background()
    projectID := core.MustParseUUID("<project-id>")
    volID := core.MustParseUUID("<volume-id>")

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
    vol, err := c.GetVolume(ctx, volID, blockstorage.GetVolumeParams{ProjectID: projectID})
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
| `ListX` | None | `(*PagedXSchema, error)` | `ListVolume(ctx, params)` returns `*PagedVolumeSchema` |
| `GetX` | None | `(*XSchema, error)` | `GetVolume(ctx, id, params)` returns `*VolumeSchema` |
| `CreateX` | `XCreateSchema` | `(*XSchema, error)` | `CreateVolume(ctx, params, body)` returns `*VolumeSchema` |
| `UpdateX` / `PartialUpdateX` | `XUpdateSchema` / `XPartialUpdateSchema` | `(*XSchema, error)` | `UpdateVolume(ctx, id, params, body)` takes `VolumePartialUpdateSchema` |
| `DeleteX` / action | A schema or none | `error` only | `RetypeVolume(ctx, id, params, body)` |

The type names come from the spec of each service, so they are not always the
same as the shapes above. For example, `server.GetServer` returns
`*ServerDetailSchema`. Read `operations.go` in the service package for the
exact signatures.

```go
// Create a volume
var createFrom blockstorage.VolumeCreateSourceOptions
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
page, err := c.ListVolume(ctx, blockstorage.ListVolumeParams{
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
    ProjectID: projectID,
}) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(item.Name)
}
```

Each iterator yields items one at a time and handles page transitions
automatically. Single-page results are available via `List<Name>`.

The generator adds an iterator only to an operation whose spec declares
`x-pagination` with `type: page_number`. That extension names the query
parameters holding the page and the page size, and the response fields holding
the items and the total, so the generator never guesses them from names. A list
answered in one response (`type: none`) or paged by cursor (`type: cursor`)
gets no iterator.

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

vol := &blockstorage.VolumeSchema{Name: "test"}
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
`core.Version` records the SDK version. Each service package also has a
generated `SpecVersion` constant (for example `server.SpecVersion`) that
records the `info.version` of the upstream OpenAPI spec it was generated from.
See [RELEASING.md](RELEASING.md).

## Development

### Prerequisites

- Go 1.26.8 or later
- Python 3 (for pipeline orchestration, normalization, facade generation)
- PyYAML: `pip install pyyaml`
- `curl` (needed only to fetch the specs from the backend)
- A `go/.env` file that sets `SPEC_BASE_URL`, copied from `.env.example`
  (needed only to fetch the specs from the backend)
- Network access to your Go module proxy (for initial `make all`)

### Make Targets

| Command | Description |
|---------|-------------|
| `make help` | Show all available targets |
| `make init` | Create `go.mod`, or sync its module path and Go version to `pipeline.yaml` |
| `make all` | Full pipeline: init + spec + generate, then tidy/build/vet. Reuses the local raw specs |
| `make all FETCH_SPEC=1` | Same, but fetch the specs from the backend first |
| `make spec` | Normalize the specs, reusing the existing `openapi.<service>.json` files |
| `make spec FETCH_SPEC=1` | Fetch the specs from the backend first, then normalize |
| `make generate` | Re-normalize each local raw spec, then generate clients + facades for all services |
| `make generate SERVICES="server network"` | Generate only the named services |
| `make build` | Compile every package in the module |
| `make vet` | Run `go vet` on every package in the module |
| `make tidy` | Run `go mod tidy` for the module |
| `make clean` | Remove generated code, facades and normalized specs. Keeps `doc.go` and `README.md` |
| `make clean-all` | Also remove `openapi.json`. The per-service raw specs `openapi.<service>.json` stay |

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
| `make pipeline-test` | Test the Python generation pipeline |
| `make test` | Run the pipeline tests, then `go test -race -cover ./...` |
| `make spec-version` | Print each service's generated `SpecVersion` |

Run `make verify` before you push. It is the same target that
[`.jenkins/check_merge_request.yml`](../.jenkins/check_merge_request.yml) runs.
The pipeline also checks the format of the merge request title, so a green
local run passes the pipeline only if the title is also correct.

### Continuous integration

| Pipeline | Trigger | What it does |
|---|---|---|
| `check_merge_request` | MR opened or reopened, new commits or a title change, or a `recheck` comment | MR title format, then `make verify` |
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
  │  • Fetch from backend (or reuse openapi.<service>.json)
  │  • Strip Django operationId prefixes
  │  • Rewrite Pydantic shapes (unions, decimals, uuids, etc)
  │  → openapi_edited.<service>.json
  │
  └─ Generate (per service)
     • oapi-codegen v2.8.0 → <service>/internal/gen/
     • Repoint runtime import to internal/oapi
     • gen_facade.py → <service>/client.go, operations.go, …
```

The spec paths, services, oapi-codegen options and normalize rules live in
`pipeline.yaml`. The backend base URL comes from `SPEC_BASE_URL` in `.env`. The
Makefile delegates to `python3 pipeline/main.py`, which reads the config and
orchestrates the steps.

### Add a New Service

1. Add the service to `pipeline.yaml` under `services:`:
   ```yaml
   - name: myservice
     folder: myservice
     tag: "My Service"
     spec:
       path: /v2/docs/api-v2/my-service.json
   ```

2. Fetch and normalize the specs. `make generate` does not fetch, and the new
   service has no raw spec yet:
   ```sh
   make spec FETCH_SPEC=1
   ```

3. Generate only the new service:
   ```sh
   make generate SERVICES="myservice" && make tidy && make build
   ```

4. Add the package to the table and the layout in the
   [repository README](../README.md).

### Regenerate After Spec Update

```sh
# Fetch from the backend in SPEC_BASE_URL (.env) and regenerate everything.
# To fetch from a different backend, change SPEC_BASE_URL in .env first.
make all FETCH_SPEC=1

# Regenerate specific services only (reuse existing spec):
make generate SERVICES="server blockstorage"
```
