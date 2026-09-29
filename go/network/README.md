# Viettel Cloud Network SDK

Package `network` manages the VPC lifecycle (create, get, update, delete) and
elastic IPs (create, get, delete).

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
    "github.com/viettelcloud-oss/sdks/go/network"
)

func main() {
    client, err := network.NewClient(
        "<api-endpoint>",
        network.WithPAT(os.Getenv("VIETTELCLOUD_PAT")),
        network.WithUserAgent("myapp/1.0.0"),
    )
    if err != nil {
        log.Fatal(err)
    }

    projectID := core.MustParseUUID(os.Getenv("VIETTELCLOUD_PROJECT_ID"))
    vpcID := core.MustParseUUID(os.Getenv("VIETTELCLOUD_VPC_ID"))

    vpc, err := client.GetVpc(
        context.Background(),
        vpcID,
        network.GetVpcParams{ProjectID: projectID},
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%+v\n", vpc)
}
```

## Operations

| Method | HTTP |
| --- | --- |
| `CreateVpc` | `POST /v2/network/vpcs/` |
| `GetVpc` | `GET /v2/network/vpcs/{vpc_id}/` |
| `UpdateVpc` | `PUT /v2/network/vpcs/{vpc_id}/` |
| `DeleteVpc` | `DELETE /v2/network/vpcs/{vpc_id}/` |
| `CreateElasticIp` | `POST /v2/network/elastic-ips/` |
| `GetElasticIp` | `GET /v2/network/elastic-ips/{eip_id}/` |
| `DeleteElasticIp` | `DELETE /v2/network/elastic-ips/{eip_id}/` |

Errors support `errors.Is` with service sentinels such as `ErrNotFound`,
`ErrUnauthorized`, `ErrValidation`, and `ErrRateLimited`.
