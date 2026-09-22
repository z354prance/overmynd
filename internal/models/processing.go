package models

import "time"

type ProcessingState string

const (
	ProcessingStateQueued     ProcessingState = "queued"
	ProcessingStateProcessing ProcessingState = "processing"
	ProcessingStateProblem    ProcessingState = "problem"
	ProcessingStateHeld       ProcessingState = "held"
)

type ProcessingJob struct {
	ID              string      `json:"id"`
	Source          ServiceType `json:"source"`
	SourceServiceID int64       `json:"source_service_id"`

	LibraryID string `json:"library_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Path      string `json:"-"`

	State ProcessingState `json:"state"`
	Stage string          `json:"stage,omitempty"`

	HealthCheck string `json:"health_check,omitempty"`
	Transcode   string `json:"transcode,omitempty"`

	NodeID   string `json:"node_id,omitempty"`
	NodeName string `json:"node_name,omitempty"`
	WorkerID string `json:"worker_id,omitempty"`

	Progress float64 `json:"progress,omitempty"`

	HoldUntil *time.Time `json:"hold_until,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`

	Message string `json:"message,omitempty"`
}
