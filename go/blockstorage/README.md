# Viettel Cloud Block Storage SDK

Package `blockstorage` manages the volume lifecycle (create, get, update,
delete) and lists the available volume types.

## Dependency setup

This package belongs to the root Viettel Cloud Go SDK module. Add the module once
by following the [module installation instructions](../README.md#installation);
no separate per-service installation is required.

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/viettelcloud-oss/sdks/go/blockstorage"
)

func main() {
    client, err := blockstorage.NewClient(
        "<api-endpoint>",
        blockstorage.WithPAT(os.Getenv("VIETTELCLOUD_PAT")),
        blockstorage.WithUserAgent("myapp/1.0.0"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // ListVolumeTypesIter pages through every volume type automatically.
    for vt, err := range client.ListVolumeTypesIter(
        context.Background(),
        blockstorage.ListVolumeTypesParams{},
    ) {
        if err != nil {
            log.Fatal(err)
        }
        fmt.Printf("%+v\n", vt)
    }
}
```

## Operations

| Method | HTTP |
| --- | --- |
| `CreateVolume` | `POST /v2/block-storage/volumes/` |
| `GetVolume` | `GET /v2/block-storage/volumes/{volume_id}/` |
| `UpdateVolume` | `PATCH /v2/block-storage/volumes/{volume_id}/` |
| `DeleteVolume` | `DELETE /v2/block-storage/volumes/{volume_id}/` |
| `ListVolumeTypes` / `ListVolumeTypesIter` | `GET /v2/block-storage/volume-types/` |

Errors support `errors.Is` with service sentinels such as `ErrNotFound`,
`ErrUnauthorized`, `ErrValidation`, and `ErrRateLimited`.
