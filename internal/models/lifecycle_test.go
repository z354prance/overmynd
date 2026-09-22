package models

import (
	"encoding/json"
	"testing"
)

func TestMediaLifecycleJSON(t *testing.T) {
	item := MediaLifecycle{
		ID:            "test-lifecycle",
		Kind:          MediaEpisode,
		Title:         "Example Show",
		SeasonNumber:  1,
		EpisodeNumber: 3,
		TVDBID:        12345,
		Stage:         LifecycleStageDownloading,
		Problems: []LifecycleProblem{
			LifecycleProblemStalled,
		},
		References: []LifecycleReference{
			{
				Source:          ServiceSonarr,
				SourceServiceID: 2,
				RecordType:      "download",
				RecordID:        "queue-1",
				Reason:          CorrelationARRID,
				Strength:        CorrelationStrong,
			},
		},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal lifecycle: %v", err)
	}

	var decoded MediaLifecycle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal lifecycle: %v", err)
	}

	if decoded.ID != item.ID {
		t.Fatalf("ID = %q, want %q", decoded.ID, item.ID)
	}

	if decoded.Stage != LifecycleStageDownloading {
		t.Fatalf(
			"Stage = %q, want %q",
			decoded.Stage,
			LifecycleStageDownloading,
		)
	}

	if len(decoded.Problems) != 1 ||
		decoded.Problems[0] != LifecycleProblemStalled {
		t.Fatalf("unexpected problems: %#v", decoded.Problems)
	}

	if len(decoded.References) != 1 {
		t.Fatalf(
			"references = %d, want 1",
			len(decoded.References),
		)
	}

	if decoded.References[0].Reason != CorrelationARRID {
		t.Fatalf(
			"reason = %q, want %q",
			decoded.References[0].Reason,
			CorrelationARRID,
		)
	}
}
