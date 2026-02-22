package core

import (
	"sync"
	"time"
)

// QuotaAmount holds a single quota amount value.
type QuotaAmount struct {
	Amount string `json:"amount"`
}

// QuotaUsage holds current and maximum quota usage.
type QuotaUsage struct {
	Current QuotaAmount `json:"current"`
	Maximum QuotaAmount `json:"maximum"`
}

// JetbrainsQuotaResponse defines the structure for the JetBrains quota API response.
type JetbrainsQuotaResponse struct {
	Current QuotaUsage `json:"current"`
	Until   string     `json:"until"`
}

// Clone returns a deep copy of JetbrainsQuotaResponse.
func (q *JetbrainsQuotaResponse) Clone() *JetbrainsQuotaResponse {
	if q == nil {
		return nil
	}
	return &JetbrainsQuotaResponse{
		Current: QuotaUsage{
			Current: QuotaAmount{Amount: q.Current.Current.Amount},
			Maximum: QuotaAmount{Amount: q.Current.Maximum.Amount},
		},
		Until: q.Until,
	}
}

// RequestStats holds aggregated request statistics for monitoring.
type RequestStats struct {
	TotalRequests      int64           `json:"total_requests"`
	SuccessfulRequests int64           `json:"successful_requests"`
	FailedRequests     int64           `json:"failed_requests"`
	TotalResponseTime  int64           `json:"total_response_time"`
	LastRequestTime    time.Time       `json:"last_request_time"`
	RequestHistory     []RequestRecord `json:"request_history"`
}

// RequestRecord represents a single request's metadata for history tracking.
type RequestRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	Success      bool      `json:"success"`
	ResponseTime int64     `json:"response_time"`
	Model        string    `json:"model"`
	Account      string    `json:"account"`
}

// PeriodStats holds computed statistics for a time period.
type PeriodStats struct {
	Requests        int64   `json:"requests"`
	SuccessRate     float64 `json:"successRate"`
	AvgResponseTime int64   `json:"avgResponseTime"`
	QPS             float64 `json:"qps"`
}

// TokenInfo holds account token and quota information for the monitoring panel.
type TokenInfo struct {
	Name       string    `json:"name"`
	License    string    `json:"license"`
	Used       float64   `json:"used"`
	Total      float64   `json:"total"`
	UsageRate  float64   `json:"usage_rate"`
	ExpiryDate time.Time `json:"expiry_date"`
	Status     string    `json:"status"`
	HasQuota   bool      `json:"has_quota"`
}

// JetbrainsAccount represents a JetBrains API account with JWT credentials.
type JetbrainsAccount struct {
	LicenseID      string     `json:"licenseId,omitempty"`
	Authorization  string     `json:"authorization,omitempty"`
	JWT            string     `json:"jwt,omitempty"` //nolint:gosec // Runtime credential field; not a hardcoded secret.
	LastUpdated    float64    `json:"last_updated"`
	HasQuota       bool       `json:"has_quota"`
	LastQuotaCheck float64    `json:"last_quota_check"`
	ExpiryTime     time.Time  `json:"expiry_time"`
	mu             sync.Mutex // account-level mutex for JWT refresh and quota check
}

// Lock acquires the account's mutex lock.
func (a *JetbrainsAccount) Lock() { a.mu.Lock() }

// Unlock releases the account's mutex lock.
func (a *JetbrainsAccount) Unlock() { a.mu.Unlock() }
