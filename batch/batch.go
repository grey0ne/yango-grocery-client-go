package batch

import (
	"context"
	"sync"

	"github.com/grey0ne/yango-grocery-client-go/client"
	"github.com/grey0ne/yango-grocery-client-go/models"
)

// BatchResult holds the aggregated result of a batched operation.
type BatchResult struct {
	Success int
	Failed  int
	Errors  []ItemError
}

// ItemError associates an error with an item identifier (e.g. product_id).
type ItemError struct {
	ItemID string
	Err    error
}

// MediaListBatchOptions configures batch media list fetching.
type MediaListBatchOptions struct {
	MaxConcurrency int // max concurrent requests (default 5)
}

// MediaListBatch fetches media for multiple product IDs with limited concurrency.
// Returns a map of productID -> media list and a BatchResult with any per-item errors.
func MediaListBatch(ctx context.Context, c *client.Client, productIDs []string, opts MediaListBatchOptions) (map[string]*models.MediaListResponse, BatchResult) {
	if c == nil {
		return nil, BatchResult{}
	}
	concurrency := opts.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	type result struct {
		productID string
		resp      *models.MediaListResponse
		err       error
	}
	ch := make(chan string, len(productIDs))
	for _, id := range productIDs {
		ch <- id
	}
	close(ch)
	var wg sync.WaitGroup
	outCh := make(chan result, len(productIDs))
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for productID := range ch {
				resp, err := c.MediaList(ctx, productID)
				outCh <- result{productID, resp, err}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(outCh)
	}()
	byProduct := make(map[string]*models.MediaListResponse)
	var resultAgg BatchResult
	for r := range outCh {
		if r.err != nil {
			resultAgg.Failed++
			resultAgg.Errors = append(resultAgg.Errors, ItemError{ItemID: r.productID, Err: r.err})
		} else {
			resultAgg.Success++
			byProduct[r.productID] = r.resp
		}
	}
	return byProduct, resultAgg
}
