package models

// LifecycleStage identifies where a media item currently sits in the
// Overmynd media pipeline.
type LifecycleStage string

const (
	LifecycleStageRequested   LifecycleStage = "requested"
	LifecycleStageWanted      LifecycleStage = "wanted"
	LifecycleStageDownloading LifecycleStage = "downloading"
	LifecycleStageDownloaded  LifecycleStage = "downloaded"
	LifecycleStageProcessing  LifecycleStage = "processing"
	LifecycleStageImporting   LifecycleStage = "importing"
	LifecycleStageAvailable   LifecycleStage = "available"
	LifecycleStagePlaying     LifecycleStage = "playing"
)

// LifecycleProblem describes an attention condition independently of the
// current pipeline stage.
type LifecycleProblem string

const (
	LifecycleProblemMissing       LifecycleProblem = "missing"
	LifecycleProblemStalled       LifecycleProblem = "stalled"
	LifecycleProblemFailed        LifecycleProblem = "failed"
	LifecycleProblemHeld          LifecycleProblem = "held"
	LifecycleProblemImportBlocked LifecycleProblem = "import_blocked"
)

// CorrelationStrength records how confidently two normalized records were
// determined to represent the same logical media item.
type CorrelationStrength string

const (
	CorrelationExact  CorrelationStrength = "exact"
	CorrelationStrong CorrelationStrength = "strong"
	CorrelationMedium CorrelationStrength = "medium"
	CorrelationWeak   CorrelationStrength = "weak"
)

// CorrelationReason records the evidence used to join records.
type CorrelationReason string

const (
	CorrelationExternalID CorrelationReason = "external_id"
	CorrelationARRID      CorrelationReason = "arr_id"
	CorrelationDownloadID CorrelationReason = "download_id"
	CorrelationPath       CorrelationReason = "path"
	CorrelationMetadata   CorrelationReason = "metadata"
)

// LifecycleReference identifies one normalized source record contributing
// evidence to a MediaLifecycle.
type LifecycleReference struct {
	Source          ServiceType `json:"source"`
	SourceServiceID int64       `json:"source_service_id"`
	RecordType      string      `json:"record_type"`
	RecordID        string      `json:"record_id"`

	Reason   CorrelationReason   `json:"reason,omitempty"`
	Strength CorrelationStrength `json:"strength,omitempty"`
}

// MediaLifecycle is Overmynd's correlated view of one logical media item.
//
// It deliberately references normalized source records rather than copying
// every service-specific field into the lifecycle.
type MediaLifecycle struct {
	ID string `json:"id"`

	Kind  MediaKind `json:"kind,omitempty"`
	Title string    `json:"title,omitempty"`
	Year  int       `json:"year,omitempty"`

	SeasonNumber  int `json:"season_number,omitempty"`
	EpisodeNumber int `json:"episode_number,omitempty"`

	TMDBID        int64  `json:"tmdb_id,omitempty"`
	TVDBID        int64  `json:"tvdb_id,omitempty"`
	IMDbID        string `json:"imdb_id,omitempty"`
	MusicBrainzID string `json:"musicbrainz_id,omitempty"`

	Stage    LifecycleStage     `json:"stage"`
	Problems []LifecycleProblem `json:"problems,omitempty"`

	References []LifecycleReference `json:"references"`
}
