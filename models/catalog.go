package models

// ProductsQueryRequest is the request body for POST /b2b/v1/products/query.
type ProductsQueryRequest struct {
	Cursor      string `json:"cursor,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	IncludeMeta bool   `json:"include_meta,omitempty"`
}

// ProductsQueryResponse is the response from products/query.
type ProductsQueryResponse struct {
	Products []Product `json:"products"`
	Cursor   string    `json:"cursor"`
}

// Product is a product from Yango API.
type Product struct {
	ProductID        string                 `json:"product_id"`
	Status           string                 `json:"status"`
	MasterCategory   string                 `json:"master_category"`
	IsMeta           bool                   `json:"is_meta"`
	CustomAttributes map[string]interface{} `json:"custom_attributes,omitempty"`
}

// MediaListRequest is the request for POST /b2b/v1/products/media/list.
type MediaListRequest struct {
	ProductID string `json:"product_id"`
}

// MediaListResponse is the response from media/list.
type MediaListResponse struct {
	Media []MediaItem `json:"media"`
}

// MediaItem is a single media entry.
type MediaItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Position string `json:"position"`
	URL      string `json:"url"`
}
