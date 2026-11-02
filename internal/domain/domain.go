// Package domain holds the core types shared by every layer. It has no
// dependencies on HTTP, storage or any framework (clean-architecture "inner ring").
package domain

import (
	"errors"
	"time"
)

type Metric string

const (
	MetricClicks        Metric = "clicks"
	MetricImpressions   Metric = "impressions"
	MetricClickToBasket Metric = "clickToBasket"
)

type EventType string

const (
	EventImpression EventType = "impression"
	EventClick      EventType = "click"
	EventAddToCart  EventType = "add_to_cart"
)

// Event is the canonical, already-validated event emitted by the ingestion tier.
// CampaignID is empty for add_to_cart: the stream processor attributes it
// to a campaign using the customer's session state (last-touch click).
type Event struct {
	ID         string
	TenantID   string
	CampaignID string
	CustomerID string
	Type       EventType
	Timestamp  time.Time // event time, not processing time
}

// Query is a request for one metric of one campaign in a time range.
type Query struct {
	TenantID   string
	CampaignID string
	Metric     Metric
	From, To   time.Time
}

// Result is what the API returns.
type Result struct {
	CampaignID string    `json:"campaignId"`
	Metric     Metric    `json:"metric"`
	Count      int64     `json:"count"` // unique customers (see Basis)
	Basis      string    `json:"basis"` // "unique_customers" | "sum_of_daily_unique"
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Source     string    `json:"source"`             // "realtime" | "historical"
	Degraded   bool      `json:"degraded,omitempty"` // true when served by fallback path
	AsOf       time.Time `json:"asOf"`
}

var (
	ErrNotFound    = errors.New("campaign not found")
	ErrInvalid     = errors.New("invalid request")
	ErrUnavailable = errors.New("backend unavailable")
)
