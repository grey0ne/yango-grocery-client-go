# yango-grocery-client-go

Reusable Go client for the Yango API. Encapsulates retries, batching, error handling, auth headers, and OpenTelemetry.

## Installation

```bash
go get github.com/grey0ne/yango-grocery-client-go
```

## Quick start

```go
package main

import (
    "context"
    "log"

    "github.com/grey0ne/yango-grocery-client-go/client"
    "github.com/grey0ne/yango-grocery-client-go/models"
)

func main() {
    c, err := client.NewClient("https://api.yango.example.com", client.WithAPIKey("your-api-key"))
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    resp, err := c.ProductsQuery(ctx, &models.ProductsQueryRequest{Limit: 100})
    if err != nil {
        log.Fatal(err)
    }
    for _, p := range resp.Products {
        log.Printf("product %s: %s", p.ProductID, p.Status)
    }
}
```

## Configuration options

```go
c, err := client.NewClient("https://api.yango.example.com",
    client.WithAPIKey("api-key"),
    client.WithTimeout(15*time.Second),
    client.WithRetries(5, 500*time.Millisecond),
    client.WithOpenTelemetry(true),
)
```

- **WithAPIKey(key)** – Sets `Authorization: Bearer <key>` and `X-API-Key`.
- **WithStaticBearerToken(token)** – Overrides Authorization (no X-API-Key).
- **WithAuthHeaderFunc(fn)** – Per-request auth from context.
- **WithDefaultUserHeaders(fn)** – Injects `X-User-Id` and `X-Customer-Id` from context.
- **WithTimeout(d)** – HTTP client timeout.
- **WithHTTPClient(cl)** – Custom `*http.Client`.
- **WithRetries(maxAttempts, delay)** – Retry count and fixed delay between attempts.
- **WithOpenTelemetry(enable)** – Wrap transport with `otelhttp`.

## Typed API and retries

Use the typed methods for automatic retries and JSON decode:

```go
resp, err := c.ProductsQuery(ctx, &models.ProductsQueryRequest{Cursor: "", Limit: 100})
if err != nil {
    return err
}

media, err := c.MediaList(ctx, productID)
if err != nil {
    return err
}
```

Retries apply to GET and to POST/PATCH when an idempotency key is set (see below).

## Raw requests (e.g. proxy)

For forwarding requests without retries (e.g. a proxy), use `RawDo`:

```go
body, statusCode, err := c.RawDo(ctx, r.Method, r.URL.Path, bodyBytes, r.Header)
if err != nil {
    // err may be *errors.Err with StatusCode and body
    return err
}
```

## Error handling

Errors are typed; use helpers to inspect them:

```go
import "github.com/grey0ne/yango-grocery-client-go/errors"

resp, err := c.ProductsQuery(ctx, req)
if err != nil {
    if errors.IsNotFound(err) {
        return nil, ErrProductNotFound
    }
    if errors.IsUnauthorized(err) {
        return nil, ErrAuth
    }
    if errors.IsRetryableError(err) {
        // consider retrying at a higher level
    }
    if e, ok := errors.AsErr(err); ok {
        log.Printf("upstream status=%d body=%s", e.StatusCode, string(e.RawBody))
    }
    return nil, err
}
```

## Batching

Fetch media for many products with limited concurrency:

```go
import "github.com/grey0ne/yango-grocery-client-go/batch"

productIDs := []string{"id1", "id2", "id3"}
mediaByProduct, result := batch.MediaListBatch(ctx, c, productIDs, batch.MediaListBatchOptions{
    MaxConcurrency: 5,
})
for id, media := range mediaByProduct {
    // use media.Media for images etc.
}
if result.Failed > 0 {
    for _, itemErr := range result.Errors {
        log.Printf("product %s: %v", itemErr.ItemID, itemErr.Err)
    }
}
```

## Idempotency

For non-idempotent methods (e.g. POST), set an idempotency key so retries are safe:

```go
import "github.com/grey0ne/yango-grocery-client-go/client"
import "github.com/grey0ne/yango-grocery-client-go/idempotency"

key, _ := idempotency.Key("catalog-sync", payload)
err := c.PostJSON(ctx, "/some/endpoint", body, &out, client.WithIdempotencyKey(key))
```

Per-request options:

- **WithIdempotencyKey(key)** – Sets `Idempotency-Key` header.
- **WithHeaders(h)** – Merges extra headers.

## User headers from context

If your auth layer puts user/customer IDs in context, plug them into the client:

```go
c, _ := client.NewClient(baseURL,
    client.WithAPIKey(apiKey),
    client.WithDefaultUserHeaders(func(ctx context.Context) (userID, customerID string) {
        if u := auth.UserFromContext(ctx); u != nil {
            return u.ID, u.CustomerID
        }
        return "", ""
    }),
)
```

Then every request will include `X-User-Id` and `X-Customer-Id` when available.
