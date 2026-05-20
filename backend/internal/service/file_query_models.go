package service

import "github.com/memodrive/backend/internal/model"

// FileQueryRequest is the unified request model for mobile category and large-list queries.
type FileQueryRequest struct {
	Category        string `json:"category,omitempty"`
	Query           string `json:"query,omitempty"`
	Sort            string `json:"sort,omitempty"`
	Cursor          string `json:"cursor,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	MediaFilter     string `json:"media_filter,omitempty"`
	DocumentSubtype string `json:"document_subtype,omitempty"`
}

// FileQueryResponse is the cursor-ready page shape shared by category and large-list queries.
type FileQueryResponse struct {
	Items      []model.File `json:"items"`
	NextCursor string       `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}

// PhotoTimelineRequest fetches one month of photo timeline data with cursor pagination.
type PhotoTimelineRequest struct {
	Year   int    `json:"year"`
	Month  int    `json:"month"`
	Query  string `json:"query,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type PhotoMonthIndexItem struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Count int `json:"count"`
}

type PhotoMonthIndexResponse struct {
	Months []PhotoMonthIndexItem `json:"months"`
}
