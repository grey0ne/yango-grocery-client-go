package client

import (
	"context"

	"github.com/grey0ne/yango-grocery-client-go/models"
)

const (
	ProductsQueryPath = "b2b/v1/products/query"
	MediaListPath     = "b2b/v1/products/media/list"
)

// ProductsQuery performs POST /b2b/v1/products/query and returns the decoded response.
func (c *Client) ProductsQuery(ctx context.Context, req *models.ProductsQueryRequest, ropts ...RequestOption) (*models.ProductsQueryResponse, error) {
	var out models.ProductsQueryResponse
	if req == nil {
		req = &models.ProductsQueryRequest{Limit: 100}
	}
	if err := c.PostJSON(ctx, ProductsQueryPath, req, &out, ropts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// MediaList performs POST /b2b/v1/products/media/list and returns media for a product.
func (c *Client) MediaList(ctx context.Context, productID string, ropts ...RequestOption) (*models.MediaListResponse, error) {
	req := &models.MediaListRequest{ProductID: productID}
	var out models.MediaListResponse
	if err := c.PostJSON(ctx, MediaListPath, req, &out, ropts...); err != nil {
		return nil, err
	}
	return &out, nil
}
