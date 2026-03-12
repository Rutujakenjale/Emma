package model

import "time"

type ImportJob struct {
	ID            string     `json:"id"`
	FileName      string     `json:"file_name"`
	Status        string     `json:"status"`
	TotalRows     int        `json:"total_rows"`
	ProcessedRows int        `json:"processed_rows"`
	SuccessCount  int        `json:"success_count"`
	FailureCount  int        `json:"failure_count"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

type PromotionCode struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	DiscountType  string    `json:"discount_type"`
	DiscountValue float64   `json:"discount_value"`
	ExpiresAt     time.Time `json:"expires_at"`
	MaxUses       int       `json:"max_uses"`
}

type ImportError struct {
	ID           string `json:"id"`
	ImportJobID  string `json:"import_job_id"`
	RowNumber    int    `json:"row_number"`
	RawData      string `json:"raw_data"`
	ErrorMessage string `json:"error_message"`
}
