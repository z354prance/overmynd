package tdarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

func TestNormalizeProcessingJobQueuedTranscode(t *testing.T) {
	now := time.UnixMilli(1800000000000).UTC()

	job, ok := normalizeProcessingJob(
		7,
		fileRecord{
			ID:                     "/media/example/movie.mkv",
			DB:                     "library1",
			HealthCheck:            "Success",
			TranscodeDecisionMaker: "Queued",
			CreatedAt:              1799990000000,
		},
		now,
	)

	if !ok {
		t.Fatal("expected queued transcode to be included")
	}

	if job.State != models.ProcessingStateQueued {
		t.Fatalf("unexpected state: %s", job.State)
	}

	if job.Stage != "transcode" {
		t.Fatalf("unexpected stage: %s", job.Stage)
	}

	if job.Title != "movie.mkv" {
		t.Fatalf("unexpected title: %s", job.Title)
	}

	if strings.Contains(job.ID, "/media/") {
		t.Fatal("processing ID exposed source path")
	}
}

func TestNormalizeProcessingJobTerminalExcluded(t *testing.T) {
	now := time.UnixMilli(1800000000000).UTC()

	_, ok := normalizeProcessingJob(
		7,
		fileRecord{
			ID:                     "/media/example/movie.mkv",
			HealthCheck:            "Success",
			TranscodeDecisionMaker: "Not required",
		},
		now,
	)

	if ok {
		t.Fatal("expected terminal record to be excluded")
	}
}

func TestNormalizeProcessingJobHeld(t *testing.T) {
	now := time.UnixMilli(1800000000000).UTC()

	job, ok := normalizeProcessingJob(
		7,
		fileRecord{
			ID:                     "/media/example/movie.mkv",
			HealthCheck:            "Success",
			TranscodeDecisionMaker: "Queued",
			HoldUntil:              now.Add(time.Hour).UnixMilli(),
		},
		now,
	)

	if !ok {
		t.Fatal("expected held record to be included")
	}

	if job.State != models.ProcessingStateHeld {
		t.Fatalf("unexpected state: %s", job.State)
	}

	if job.HoldUntil == nil {
		t.Fatal("expected hold_until")
	}
}

func TestProblemState(t *testing.T) {
	for _, value := range []string{
		"Error",
		"Failed",
		"Unhealthy",
		"Transcode Error",
	} {
		if !isProblemState(value) {
			t.Fatalf("expected %q to be a problem state", value)
		}
	}

	for _, value := range []string{
		"",
		"Success",
		"Queued",
		"Not required",
	} {
		if isProblemState(value) {
			t.Fatalf("did not expect %q to be a problem state", value)
		}
	}
}

func TestNormalizeActiveTranscodeWorker(t *testing.T) {
	job, ok := normalizeActiveWorker(
		7,
		"idHvxZy",
		"Transcode_self2",
		"tan-teal",
		workerRecord{
			ID:         "tan-teal",
			WorkerType: "transcodegpu",
			Idle:       false,
			File:       "/mnt/media/Downloads/Test Show/Test.Show.S01E01.mkv",
			Percentage: 42.5,
			FPS:        61.2,
			ETA:        "00:04:12",
			Status:     "Transcoding",
			Job: workerJob{
				Version:     "2.89.01",
				FootprintID: "GZFuTUH",
				JobID:       "w2kejHa",
				Start:       1790049838873,
				Type:        "transcode",
			},
		},
	)

	if !ok {
		t.Fatal("active worker was excluded")
	}

	if job.State != models.ProcessingStateProcessing {
		t.Fatalf("state = %q, want processing", job.State)
	}

	if job.Stage != "transcode" {
		t.Fatalf("stage = %q, want transcode", job.Stage)
	}

	if job.NodeID != "idHvxZy" {
		t.Fatalf("node ID = %q", job.NodeID)
	}

	if job.NodeName != "Transcode_self2" {
		t.Fatalf("node name = %q", job.NodeName)
	}

	if job.WorkerID != "tan-teal" {
		t.Fatalf("worker ID = %q", job.WorkerID)
	}

	if job.Progress != 42.5 {
		t.Fatalf("progress = %v, want 42.5", job.Progress)
	}

	if job.Title != "Test.Show.S01E01.mkv" {
		t.Fatalf("title = %q", job.Title)
	}

	if job.Path != "/mnt/media/Downloads/Test Show/Test.Show.S01E01.mkv" {
		t.Fatalf("path = %q", job.Path)
	}

	if job.Transcode != "Transcoding" {
		t.Fatalf("transcode = %q", job.Transcode)
	}

	if job.Message != "Transcoding · 61.2 FPS · ETA 00:04:12" {
		t.Fatalf("message = %q", job.Message)
	}

	if job.CreatedAt == nil {
		t.Fatal("worker start time missing")
	}
}

func TestNormalizeActiveHealthCheckWorker(t *testing.T) {
	job, ok := normalizeActiveWorker(
		7,
		"node-one",
		"Tdarr Node",
		"worker-one",
		workerRecord{
			WorkerType: "healthcheckcpu",
			Idle:       false,
			File:       "/media/Movie.mkv",
			Status:     "Scanning",
			Job: workerJob{
				Type: "healthcheck",
			},
		},
	)

	if !ok {
		t.Fatal("active health-check worker was excluded")
	}

	if job.State != models.ProcessingStateProcessing {
		t.Fatalf("state = %q, want processing", job.State)
	}

	if job.Stage != "health_check" {
		t.Fatalf("stage = %q, want health_check", job.Stage)
	}

	if job.HealthCheck != "Scanning" {
		t.Fatalf("health check = %q", job.HealthCheck)
	}

	if job.WorkerID != "worker-one" {
		t.Fatalf("worker fallback ID = %q", job.WorkerID)
	}
}

func TestNormalizeActiveWorkerIgnoresIdle(t *testing.T) {
	_, ok := normalizeActiveWorker(
		7,
		"node-one",
		"Tdarr Node",
		"worker-one",
		workerRecord{
			Idle: true,
			File: "/media/Movie.mkv",
		},
	)

	if ok {
		t.Fatal("idle worker should be excluded")
	}
}

func TestNormalizeActiveWorkerRequiresFile(t *testing.T) {
	_, ok := normalizeActiveWorker(
		7,
		"node-one",
		"Tdarr Node",
		"worker-one",
		workerRecord{
			Idle: false,
		},
	)

	if ok {
		t.Fatal("worker without file should be excluded")
	}
}

func TestActiveWorkerStage(t *testing.T) {
	tests := []struct {
		workerType string
		jobType    string
		want       string
	}{
		{"transcodegpu", "transcode", "transcode"},
		{"transcodecpu", "transcode", "transcode"},
		{"healthcheckcpu", "healthcheck", "health_check"},
		{"healthcheckgpu", "", "health_check"},
	}

	for _, test := range tests {
		got := activeWorkerStage(test.workerType, test.jobType)
		if got != test.want {
			t.Fatalf(
				"activeWorkerStage(%q, %q) = %q, want %q",
				test.workerType,
				test.jobType,
				got,
				test.want,
			)
		}
	}
}

func TestProcessingPathKeyNormalizesPath(t *testing.T) {
	got := processingPathKey(`C:\Media\Test Show\Episode.mkv`)
	want := "c:/media/test show/episode.mkv"

	if got != want {
		t.Fatalf("processingPathKey = %q, want %q", got, want)
	}
}

func TestClampProgress(t *testing.T) {
	if got := clampProgress(-10); got != 0 {
		t.Fatalf("negative progress = %v", got)
	}

	if got := clampProgress(47.25); got != 47.25 {
		t.Fatalf("normal progress = %v", got)
	}

	if got := clampProgress(120); got != 100 {
		t.Fatalf("over-100 progress = %v", got)
	}
}

func TestProcessingRetainsQueueWhenGetNodesFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/cruddb":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[
					{
						"_id": "/media/Test.Show.S01E01.mkv",
						"DB": "library1",
						"HealthCheck": "Success",
						"TranscodeDecisionMaker": "Queued",
						"createdAt": 1790049838873
					}
				]`))

			case "/api/v2/get-nodes":
				http.Error(
					w,
					"temporary node failure",
					http.StatusInternalServerError,
				)

			default:
				http.NotFound(w, r)
			}
		},
	))
	defer server.Close()

	integration := &Integration{}

	jobs, err := integration.Processing(
		context.Background(),
		models.Service{
			ID:      7,
			Type:    models.ServiceTdarr,
			BaseURL: server.URL,
		},
		"",
	)
	if err != nil {
		t.Fatalf(
			"Processing returned error when get-nodes failed: %v",
			err,
		)
	}

	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}

	job := jobs[0]

	if job.State != models.ProcessingStateQueued {
		t.Fatalf("state = %q, want queued", job.State)
	}

	if job.Stage != "transcode" {
		t.Fatalf("stage = %q, want transcode", job.Stage)
	}

	if job.Title != "Test.Show.S01E01.mkv" {
		t.Fatalf("title = %q", job.Title)
	}

	if job.LibraryID != "library1" {
		t.Fatalf("library ID = %q", job.LibraryID)
	}
}
