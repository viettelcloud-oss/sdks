# Viettel Cloud SDKs

Official client SDKs for the Viettel Cloud API.

Go is the first supported language. Its packages are distributed together as
one module:

```text
github.com/viettelcloud-oss/sdks/go
```

## Go packages

| Package | Import path | Description |
| --- | --- | --- |
| Server | `github.com/viettelcloud-oss/sdks/go/server` | Virtual server lifecycle and related resources |
| Network | `github.com/viettelcloud-oss/sdks/go/network` | Networks, subnets, routers and related resources |
| Block Storage | `github.com/viettelcloud-oss/sdks/go/blockstorage` | Volumes, snapshots, backups and related resources |
| Core | `github.com/viettelcloud-oss/sdks/go/core` | Shared options, errors, retry and pagination primitives |

Generated HTTP bindings live under each service's `internal/gen` package.
Consumers use only the public service facades. Shared SDK behavior in `go/core`
is maintained by hand and is not rewritten by service code generation.

## Installation

Add the module to your project with a release tag:

```sh
go get github.com/viettelcloud-oss/sdks/go@latest
```

Release tags use the format `go/vX.Y.Z` because the module lives in the `go/`
subdirectory. Go reads the tag `go/v0.1.0` as the module version `v0.1.0`:

```sh
go get github.com/viettelcloud-oss/sdks/go@v0.1.0
```

If an application imports only one service package, it still gets the one
module version that all service packages share.

### Developing against a local checkout

To test an unreleased SDK change in a consumer, clone both repositories side by
side and create a Go workspace in their parent directory:

```sh
go work init ./<consumer>
go work edit -replace=github.com/viettelcloud-oss/sdks/go=./sdks/go
```

Go then builds the consumer with the local SDK checkout. Neither `go.mod`
changes. Use `replace` in `go.work`, not `go work use ./sdks/go`. If the consumer
requires an SDK version that is not released yet, `use` fails: Go still
downloads the `go.mod` of that version. Do not commit
`go.work` or `go.work.sum`.

## Quick start

```go
package main

import (
    "log"

    "github.com/viettelcloud-oss/sdks/go/blockstorage"
    "github.com/viettelcloud-oss/sdks/go/core"
)

func main() {
    client, err := blockstorage.NewClient(
        "<api-endpoint>",
        blockstorage.WithPAT("your-personal-access-token"),
        blockstorage.WithRetry(core.DefaultRetry()),
        blockstorage.WithUserAgent("myapp/1.0.0"),
    )
    if err != nil {
        log.Fatal(err)
    }

    _ = client
}
```

Authentication in v1 uses Personal Access Tokens only. The SDK sends:

```text
Authorization: Token <token>
```

See [`go/README.md`](go/README.md) and the README in each service package for
typed operations, errors, pagination, retries and client options.

## Versioning and releases

The Go SDK follows Semantic Versioning. All Go packages are released together
under one version.

Because the Go module is located in the repository's `go/` subdirectory, Git
release tags use the subdirectory prefix:

```text
go/v0.1.0
```

Consumers select a release by its module version:

```sh
go get github.com/viettelcloud-oss/sdks/go@v0.1.0
```

Releases are cut by the `release` Jenkins pipeline, which runs semantic-release
over the conventional-commit titles since the last tag. Release notes record
both the SDK version and the upstream OpenAPI `info.version`. Generated
bindings and facades are committed, so consumers do not need the OpenAPI
specification or code-generation tools.

## Repository layout

```text
.jenkins/                 # CI pipelines (merge-request and main checks, release)
.releaserc                # semantic-release config; tags as go/vX.Y.Z
tools/                    # Pinned dev tools, kept out of the published module
go/
├── go.mod
├── core/
├── server/
│   └── internal/gen/
├── network/
│   └── internal/gen/
├── project/
│   └── internal/gen/
└── blockstorage/
    └── internal/gen/
```

The repository uses a single root Go module and does not publish a `go.work`
file. `tools/` is a second, separate module so the ~200 dependencies of
`golangci-lint`, `gofumpt` and `goimports` stay out of `go/go.mod` and never
become transitive requirements of the SDK. The directory is still part of the
repository, but `go get` downloads only the `go/` module, so consumers never
build it.

## Development

Run `make -C go verify` before pushing; it is exactly what the merge-request
pipeline runs.

Go SDK generation, validation, CI and release instructions are documented in:

- [`go/README.md`](go/README.md)
- [`go/RELEASING.md`](go/RELEASING.md)

## License

TBD
