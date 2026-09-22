package tdarr

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

type crudDBRequest struct {
	Data crudDBRequestData `json:"data"`
}

type crudDBRequestData struct {
	Collection string `json:"collection"`
	Mode       string `json:"mode"`
	DocID      string `json:"docID"`
	Obj        any    `json:"obj"`
}

type fileRecord struct {
	ID                     string `json:"_id"`
	DB                     string `json:"DB"`
	HealthCheck            string `json:"HealthCheck"`
	TranscodeDecisionMaker string `json:"TranscodeDecisionMaker"`
	HoldUntil              int64  `json:"holdUntil"`
	Bumped                 bool   `json:"bumped"`
	CreatedAt              int64  `json:"createdAt"`
}

type nodeRecord struct {
	ID       string                  `json:"_id"`
	NodeName string                  `json:"nodeName"`
	Workers  map[string]workerRecord `json:"workers"`
}

type workerRecord struct {
	ID           string    `json:"_id"`
	WorkerType   string    `json:"workerType"`
	Created      bool      `json:"created"`
	Idle         bool      `json:"idle"`
	IsFlowWorker bool      `json:"isFlowWorker"`
	File         string    `json:"file"`
	Percentage   float64   `json:"percentage"`
	FPS          float64   `json:"fps"`
	ETA          string    `json:"ETA"`
	Status       string    `json:"status"`
	StatusTs     int64     `json:"statusTs"`
	Job          workerJob `json:"job"`
}

type workerJob struct {
	Version     string `json:"version"`
	FootprintID string `json:"footprintId"`
	JobID       string `json:"jobId"`
	Start       int64  `json:"start"`
	Type        string `json:"type"`
	FileID      string `json:"fileId"`
}

func (i *Integration) Processing(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.ProcessingJob, error) {
	client := NewClient()

	var records []fileRecord

	payload := crudDBRequest{
		Data: crudDBRequestData{
			Collection: "FileJSONDB",
			Mode:       "getAll",
			DocID:      "",
			Obj:        map[string]any{},
		},
	}

	if err := client.PostJSON(
		ctx,
		service.BaseURL,
		"cruddb",
		payload,
		&records,
	); err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	jobsByPath := make(map[string]models.ProcessingJob)
	unkeyedJobs := make([]models.ProcessingJob, 0)

	for _, record := range records {
		job, ok := normalizeProcessingJob(
			service.ID,
			record,
			now,
		)
		if !ok {
			continue
		}

		key := processingPathKey(job.Path)
		if key == "" {
			unkeyedJobs = append(unkeyedJobs, job)
			continue
		}

		jobsByPath[key] = job
	}

	var nodes map[string]nodeRecord

	// Active-worker data enriches the FileJSONDB queue. If get-nodes
	// is temporarily unavailable, retain the queued/held/problem jobs
	// already collected above rather than failing Processing entirely.
	if err := client.GetJSON(
		ctx,
		service.BaseURL,
		"get-nodes",
		&nodes,
	); err == nil {
		for nodeKey, node := range nodes {
			nodeID := strings.TrimSpace(node.ID)
			if nodeID == "" {
				nodeID = strings.TrimSpace(nodeKey)
			}

			for workerKey, worker := range node.Workers {
				job, ok := normalizeActiveWorker(
					service.ID,
					nodeID,
					node.NodeName,
					workerKey,
					worker,
				)
				if !ok {
					continue
				}

				key := processingPathKey(job.Path)
				if key == "" {
					unkeyedJobs = append(unkeyedJobs, job)
					continue
				}

				// Active work is more authoritative than a queued FileJSONDB
				// record for the same file.
				jobsByPath[key] = job
			}
		}
	}

	jobs := make([]models.ProcessingJob, 0, len(jobsByPath)+len(unkeyedJobs))

	for _, job := range jobsByPath {
		jobs = append(jobs, job)
	}

	jobs = append(jobs, unkeyedJobs...)

	return jobs, nil
}

func normalizeProcessingJob(
	serviceID int64,
	record fileRecord,
	now time.Time,
) (models.ProcessingJob, bool) {
	health := strings.TrimSpace(record.HealthCheck)
	transcode := strings.TrimSpace(record.TranscodeDecisionMaker)

	state, stage, attention := processingState(
		health,
		transcode,
		record.HoldUntil,
		now,
	)
	if !attention {
		return models.ProcessingJob{}, false
	}

	job := models.ProcessingJob{
		ID:              processingID(serviceID, record.ID),
		Source:          models.ServiceTdarr,
		SourceServiceID: serviceID,
		LibraryID:       record.DB,
		Title:           filepath.Base(record.ID),
		Path:            record.ID,
		State:           state,
		Stage:           stage,
		HealthCheck:     health,
		Transcode:       transcode,
	}

	if record.CreatedAt > 0 {
		created := time.UnixMilli(record.CreatedAt).UTC()
		job.CreatedAt = &created
	}

	if record.HoldUntil > now.UnixMilli() {
		holdUntil := time.UnixMilli(record.HoldUntil).UTC()
		job.HoldUntil = &holdUntil
	}

	return job, true
}

func normalizeActiveWorker(
	serviceID int64,
	nodeID string,
	nodeName string,
	workerKey string,
	worker workerRecord,
) (models.ProcessingJob, bool) {
	if worker.Idle {
		return models.ProcessingJob{}, false
	}

	path := strings.TrimSpace(worker.File)
	if path == "" {
		return models.ProcessingJob{}, false
	}

	workerID := strings.TrimSpace(worker.ID)
	if workerID == "" {
		workerID = strings.TrimSpace(workerKey)
	}

	stage := activeWorkerStage(worker.WorkerType, worker.Job.Type)

	job := models.ProcessingJob{
		ID:              processingID(serviceID, path),
		Source:          models.ServiceTdarr,
		SourceServiceID: serviceID,
		Title:           filepath.Base(path),
		Path:            path,
		State:           models.ProcessingStateProcessing,
		Stage:           stage,
		NodeID:          strings.TrimSpace(nodeID),
		NodeName:        strings.TrimSpace(nodeName),
		WorkerID:        workerID,
		Progress:        clampProgress(worker.Percentage),
		Message:         activeWorkerMessage(worker),
	}

	if stage == "health_check" {
		job.HealthCheck = strings.TrimSpace(worker.Status)
	} else {
		job.Transcode = strings.TrimSpace(worker.Status)
	}

	if worker.Job.Start > 0 {
		started := time.UnixMilli(worker.Job.Start).UTC()
		job.CreatedAt = &started
	}

	return job, true
}

func activeWorkerStage(workerType string, jobType string) string {
	combined := strings.ToLower(
		strings.TrimSpace(workerType) + " " + strings.TrimSpace(jobType),
	)

	if strings.Contains(combined, "health") {
		return "health_check"
	}

	return "transcode"
}

func activeWorkerMessage(worker workerRecord) string {
	parts := make([]string, 0, 3)

	if status := strings.TrimSpace(worker.Status); status != "" {
		parts = append(parts, status)
	}

	if worker.FPS > 0 {
		parts = append(parts, fmt.Sprintf("%.1f FPS", worker.FPS))
	}

	if eta := strings.TrimSpace(worker.ETA); eta != "" &&
		!strings.EqualFold(eta, "Calc...") {
		parts = append(parts, "ETA "+eta)
	}

	return strings.Join(parts, " · ")
}

func clampProgress(progress float64) float64 {
	if progress < 0 {
		return 0
	}

	if progress > 100 {
		return 100
	}

	return progress
}

func processingPathKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimRight(path, "/")

	return strings.ToLower(path)
}

func processingState(
	health string,
	transcode string,
	holdUntil int64,
	now time.Time,
) (models.ProcessingState, string, bool) {
	if holdUntil > now.UnixMilli() {
		return models.ProcessingStateHeld, "held", true
	}

	if strings.EqualFold(health, "Queued") {
		return models.ProcessingStateQueued, "health_check", true
	}

	if strings.EqualFold(transcode, "Queued") {
		return models.ProcessingStateQueued, "transcode", true
	}

	if isProblemState(health) {
		return models.ProcessingStateProblem, "health_check", true
	}

	if isProblemState(transcode) {
		return models.ProcessingStateProblem, "transcode", true
	}

	return "", "", false
}

func isProblemState(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))

	if value == "" {
		return false
	}

	for _, token := range []string{
		"error",
		"fail",
		"unhealthy",
	} {
		if strings.Contains(value, token) {
			return true
		}
	}

	return false
}

func processingID(serviceID int64, sourceID string) string {
	sum := sha256.Sum256([]byte(sourceID))

	return fmt.Sprintf(
		"%d:tdarr:%x",
		serviceID,
		sum[:12],
	)
}
