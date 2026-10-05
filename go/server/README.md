# Viettel Cloud Server SDK

Package `server` manages the virtual server lifecycle: create, get, partial
update, and delete.

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

    "github.com/viettelcloud-oss/sdks/go/core"
    "github.com/viettelcloud-oss/sdks/go/server"
)

func main() {
    client, err := server.NewClient(
        "<api-endpoint>",
        server.WithPAT(os.Getenv("VIETTELCLOUD_PAT")),
        server.WithUserAgent("myapp/1.0.0"),
    )
    if err != nil {
        log.Fatal(err)
    }

    projectID := core.MustParseUUID(os.Getenv("VIETTELCLOUD_PROJECT_ID"))
    serverID := core.MustParseUUID(os.Getenv("VIETTELCLOUD_SERVER_ID"))

    srv, err := client.GetServer(
        context.Background(),
        serverID,
        server.GetServerParams{ProjectID: projectID},
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%+v\n", srv)
}
```

## Operations

| Method | HTTP |
| --- | --- |
| `CreateServer` | `POST /v2/server/servers/` |
| `GetServer` | `GET /v2/server/servers/{server_id}/` |
| `PartialUpdateServer` | `PATCH /v2/server/servers/{server_id}/` |
| `DeleteServer` | `DELETE /v2/server/servers/{server_id}/` |

Errors support `errors.Is` with service sentinels such as `ErrNotFound`,
`ErrUnauthorized`, `ErrValidation`, and `ErrRateLimited`.
