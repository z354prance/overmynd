package correlation

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/z354prance/overmynd/internal/models"
)

// Input contains normalized records from Overmynd's service layer.
type Input struct {
	Attention  []models.AttentionItem
	Requests   []models.Request
	Downloads  []models.Download
	Processing []models.ProcessingJob
	Playback   []models.PlaybackSession
}

type record struct {
	recordType string
	recordID   string

	source          models.ServiceType
	sourceServiceID int64

	identity Identity
	metadata MetadataIdentity

	download   *models.Download
	processing *models.ProcessingJob

	title string
	year  int

	seasonNumber  int
	episodeNumber int

	stage    models.LifecycleStage
	problems []models.LifecycleProblem
}

// Engine correlates normalized service records into media lifecycles.
type Engine struct{}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Build(input Input) []models.MediaLifecycle {
	records := flatten(input)
	if len(records) == 0 {
		return []models.MediaLifecycle{}
	}

	parent := make([]int, len(records))
	for i := range parent {
		parent[i] = i
	}

	var reasons = make(map[[2]int]Match)

	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			match := matchRecords(records[i], records[j])
			if !match.Matched {
				continue
			}

			if match.Strength == models.CorrelationWeak &&
				!allowWeakUnion(records[i], records[j]) {
				continue
			}

			union(parent, i, j)
			reasons[[2]int{i, j}] = match
		}
	}

	groups := make(map[int][]int)
	for i := range records {
		root := find(parent, i)
		groups[root] = append(groups[root], i)
	}

	lifecycles := make([]models.MediaLifecycle, 0, len(groups))

	for _, indexes := range groups {
		lifecycles = append(
			lifecycles,
			buildLifecycle(records, indexes, reasons),
		)
	}

	sort.Slice(lifecycles, func(i, j int) bool {
		return lifecycles[i].ID < lifecycles[j].ID
	})

	return lifecycles
}

func flatten(input Input) []record {
	records := make(
		[]record,
		0,
		len(input.Attention)+
			len(input.Requests)+
			len(input.Downloads)+
			len(input.Processing)+
			len(input.Playback),
	)

	for i := range input.Attention {
		item := input.Attention[i]

		problems := make([]models.LifecycleProblem, 0, 1)
		stage := models.LifecycleStageWanted

		switch item.State {
		case models.AttentionMissing:
			problems = append(problems, models.LifecycleProblemMissing)
		case models.AttentionProblem:
			problems = append(problems, models.LifecycleProblemFailed)
		case models.AttentionInProgress:
			stage = models.LifecycleStageDownloading
		}

		records = append(records, record{
			recordType:      "attention",
			recordID:        item.ID,
			source:          item.Source,
			sourceServiceID: item.SourceServiceID,
			identity:        IdentityFromAttention(item),
			metadata:        MetadataFromAttention(item),
			title:           item.Title,
			year:            item.Year,
			seasonNumber:    item.SeasonNumber,
			episodeNumber:   item.EpisodeNumber,
			stage:           stage,
			problems:        problems,
		})
	}

	for i := range input.Requests {
		item := input.Requests[i]

		records = append(records, record{
			recordType:      "request",
			recordID:        item.ID,
			source:          item.Source,
			sourceServiceID: item.SourceServiceID,
			identity:        IdentityFromRequest(item),
			title:           item.Title,
			stage:           models.LifecycleStageRequested,
		})
	}

	for i := range input.Downloads {
		item := input.Downloads[i]

		stage := downloadStage(item)
		problems := downloadProblems(item)

		records = append(records, record{
			recordType:      "download",
			recordID:        item.ID,
			source:          item.Source,
			sourceServiceID: item.SourceServiceID,
			identity:        IdentityFromDownload(item),
			metadata:        MetadataFromDownload(item),
			download:        &input.Downloads[i],
			title:           item.Title,
			stage:           stage,
			problems:        problems,
		})
	}

	for i := range input.Processing {
		item := input.Processing[i]

		problems := make([]models.LifecycleProblem, 0, 1)
		if item.State == models.ProcessingStateHeld {
			problems = append(problems, models.LifecycleProblemHeld)
		}
		if item.State == models.ProcessingStateProblem {
			if item.Source == models.ServiceType("tdarr") && item.Stage == "health_check" {
				problems = append(problems, models.LifecycleProblemHealthCheckFailed)
			} else {
				problems = append(problems, models.LifecycleProblemFailed)
			}
		}

		records = append(records, record{
			recordType:      "processing",
			recordID:        item.ID,
			source:          item.Source,
			sourceServiceID: item.SourceServiceID,
			metadata:        MetadataFromProcessing(item),
			processing:      &input.Processing[i],
			title:           item.Title,
			stage:           models.LifecycleStageProcessing,
			problems:        problems,
		})
	}

	for i := range input.Playback {
		item := input.Playback[i]

		records = append(records, record{
			recordType:      "playback",
			recordID:        item.ID,
			source:          item.Source,
			sourceServiceID: item.SourceServiceID,
			identity:        IdentityFromPlayback(item),
			metadata:        MetadataFromPlayback(item),
			title:           playbackTitle(item),
			year:            item.Year,
			seasonNumber:    item.SeasonNumber,
			episodeNumber:   item.EpisodeNumber,
			stage:           models.LifecycleStagePlaying,
		})
	}

	return records
}

func matchRecords(a, b record) Match {
	if match := MatchIdentity(a.identity, b.identity); match.Matched {
		return match
	}

	if a.download != nil && b.download != nil {
		if match := MatchDownloadIdentity(*a.download, *b.download); match.Matched {
			return match
		}
	}

	if a.download != nil && b.processing != nil {
		if match := MatchDownloadProcessingPath(
			*a.download,
			*b.processing,
		); match.Matched {
			return match
		}
	}

	if b.download != nil && a.processing != nil {
		if match := MatchDownloadProcessingPath(
			*b.download,
			*a.processing,
		); match.Matched {
			return match
		}
	}

	return MatchMetadata(a.metadata, b.metadata)
}

func buildLifecycle(
	records []record,
	indexes []int,
	reasons map[[2]int]Match,
) models.MediaLifecycle {
	lifecycle := models.MediaLifecycle{
		Stage:      models.LifecycleStageRequested,
		Problems:   []models.LifecycleProblem{},
		References: []models.LifecycleReference{},
	}

	stageRank := -1
	problemSet := make(map[models.LifecycleProblem]struct{})

	for _, index := range indexes {
		item := records[index]

		enrichLifecycle(&lifecycle, item)

		if rank := lifecycleStageRank(item.stage); rank > stageRank {
			lifecycle.Stage = item.stage
			stageRank = rank
		}

		for _, problem := range item.problems {
			problemSet[problem] = struct{}{}
		}

		reason, strength := correlationEvidence(
			index,
			indexes,
			reasons,
		)

		lifecycle.References = append(
			lifecycle.References,
			models.LifecycleReference{
				Source:          item.source,
				SourceServiceID: item.sourceServiceID,
				RecordType:      item.recordType,
				RecordID:        item.recordID,
				Reason:          reason,
				Strength:        strength,
			},
		)
	}

	// A matched active/queued Tdarr job explains why ARR is still waiting to
	// import. Preserve other problems and restore the warning once processing ends.
	for _, index := range indexes {
		job := records[index].processing
		if job != nil && (job.State == models.ProcessingStateProcessing || job.State == models.ProcessingStateQueued) && lifecycle.Stage == models.LifecycleStageImporting {
			lifecycle.Stage = models.LifecycleStageProcessing
			delete(problemSet, models.LifecycleProblemImportBlocked)
			break
		}
	}
	for problem := range problemSet {
		lifecycle.Problems = append(lifecycle.Problems, problem)
	}

	sort.Slice(lifecycle.Problems, func(i, j int) bool {
		return lifecycle.Problems[i] < lifecycle.Problems[j]
	})

	sort.Slice(lifecycle.References, func(i, j int) bool {
		a := lifecycle.References[i]
		b := lifecycle.References[j]

		if a.RecordType != b.RecordType {
			return a.RecordType < b.RecordType
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.RecordID < b.RecordID
	})

	lifecycle.ID = lifecycleID(lifecycle)

	return lifecycle
}

func enrichLifecycle(
	lifecycle *models.MediaLifecycle,
	item record,
) {
	if lifecycle.Kind == "" && item.identity.Kind != "" {
		lifecycle.Kind = item.identity.Kind
	}

	if lifecycle.Title == "" && item.title != "" {
		lifecycle.Title = item.title
	}

	if lifecycle.Year == 0 && item.year != 0 {
		lifecycle.Year = item.year
	}

	if lifecycle.SeasonNumber == 0 && item.seasonNumber != 0 {
		lifecycle.SeasonNumber = item.seasonNumber
	}

	if lifecycle.EpisodeNumber == 0 && item.episodeNumber != 0 {
		lifecycle.EpisodeNumber = item.episodeNumber
	}

	if lifecycle.TMDBID == 0 && item.identity.TMDBID != 0 {
		lifecycle.TMDBID = item.identity.TMDBID
	}

	if lifecycle.TVDBID == 0 && item.identity.TVDBID != 0 {
		lifecycle.TVDBID = item.identity.TVDBID
	}

	if lifecycle.IMDbID == "" && item.identity.IMDbID != "" {
		lifecycle.IMDbID = item.identity.IMDbID
	}

	if lifecycle.MusicBrainzID == "" &&
		item.identity.MusicBrainzID != "" {
		lifecycle.MusicBrainzID = item.identity.MusicBrainzID
	}
}

func correlationEvidence(
	index int,
	group []int,
	reasons map[[2]int]Match,
) (models.CorrelationReason, models.CorrelationStrength) {
	bestRank := -1
	var best Match

	for _, other := range group {
		if index == other {
			continue
		}

		key := [2]int{index, other}
		if index > other {
			key = [2]int{other, index}
		}

		match, ok := reasons[key]
		if !ok {
			continue
		}

		rank := correlationStrengthRank(match.Strength)
		if rank > bestRank {
			bestRank = rank
			best = match
		}
	}

	return best.Reason, best.Strength
}

func downloadStage(item models.Download) models.LifecycleStage {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	tracked := strings.ToLower(strings.TrimSpace(item.TrackedDownload))

	if status == "completed" && strings.Contains(tracked, "warning") {
		return models.LifecycleStageImporting
	}

	if item.Size > 0 && item.SizeLeft == 0 {
		return models.LifecycleStageDownloaded
	}

	return models.LifecycleStageDownloading
}

func downloadProblems(item models.Download) []models.LifecycleProblem {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	tracked := strings.ToLower(strings.TrimSpace(item.TrackedDownload))

	var problems []models.LifecycleProblem

	if strings.Contains(status, "stall") {
		problems = append(problems, models.LifecycleProblemStalled)
	}

	if status == "completed" && strings.Contains(tracked, "warning") {
		problems = append(problems, models.LifecycleProblemImportBlocked)
		return problems
	}

	for _, value := range []string{status, tracked} {
		if strings.Contains(value, "fail") ||
			strings.Contains(value, "error") ||
			strings.Contains(value, "warning") {
			problems = append(problems, models.LifecycleProblemFailed)
			break
		}
	}

	return problems
}

func playbackTitle(item models.PlaybackSession) string {
	if strings.EqualFold(item.MediaType, "episode") &&
		item.ShowTitle != "" {
		return item.ShowTitle
	}

	return item.MediaTitle
}

func lifecycleStageRank(stage models.LifecycleStage) int {
	switch stage {
	case models.LifecycleStageRequested:
		return 0
	case models.LifecycleStageWanted:
		return 1
	case models.LifecycleStageDownloading:
		return 2
	case models.LifecycleStageDownloaded:
		return 3
	case models.LifecycleStageProcessing:
		return 4
	case models.LifecycleStageImporting:
		return 5
	case models.LifecycleStageAvailable:
		return 6
	case models.LifecycleStagePlaying:
		return 7
	default:
		return -1
	}
}

func correlationStrengthRank(
	strength models.CorrelationStrength,
) int {
	switch strength {
	case models.CorrelationExact:
		return 3
	case models.CorrelationStrong:
		return 2
	case models.CorrelationMedium:
		return 1
	case models.CorrelationWeak:
		return 0
	default:
		return -1
	}
}

func lifecycleID(item models.MediaLifecycle) string {
	identity := ""

	switch {
	case item.TMDBID != 0:
		identity = fmt.Sprintf("tmdb:%d", item.TMDBID)
	case item.TVDBID != 0:
		identity = fmt.Sprintf(
			"tvdb:%d:s%d:e%d",
			item.TVDBID,
			item.SeasonNumber,
			item.EpisodeNumber,
		)
	case item.IMDbID != "":
		identity = "imdb:" + item.IMDbID
	case item.MusicBrainzID != "":
		identity = "musicbrainz:" + item.MusicBrainzID
	default:
		identity = fmt.Sprintf(
			"%s:%s:%d:%d:%d",
			item.Kind,
			normalizeTitle(item.Title),
			item.Year,
			item.SeasonNumber,
			item.EpisodeNumber,
		)
	}

	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("lifecycle:%x", sum[:12])
}

func find(parent []int, value int) int {
	if parent[value] != value {
		parent[value] = find(parent, parent[value])
	}
	return parent[value]
}

func union(parent []int, a, b int) {
	rootA := find(parent, a)
	rootB := find(parent, b)

	if rootA != rootB {
		parent[rootB] = rootA
	}
}

// allowWeakUnion limits fallback correlation to records where weak metadata or
// filename evidence is useful without allowing two authoritative media
// identities to be merged solely because they share a title or filename.
func allowWeakUnion(a, b record) bool {
	aAuthoritative := hasAuthoritativeIdentity(a.identity)
	bAuthoritative := hasAuthoritativeIdentity(b.identity)

	// If both records already carry authoritative identities, a weak fallback
	// must never override the fact that stronger evidence failed to match them.
	if aAuthoritative && bAuthoritative {
		return false
	}

	// Processing records often have only a path/title, so weak path evidence is
	// allowed to attach them to an identified download or media record.
	if a.processing != nil || b.processing != nil {
		return true
	}

	// Downloader records may lack media IDs. Allow a weak fallback only when at
	// least one side does not already have authoritative media identity.
	if a.download != nil || b.download != nil {
		return !aAuthoritative || !bAuthoritative
	}

	return false
}

func hasAuthoritativeIdentity(identity Identity) bool {
	return identity.TMDBID != 0 ||
		identity.TVDBID != 0 ||
		identity.IMDbID != "" ||
		identity.MusicBrainzID != "" ||
		identity.MovieID != 0 ||
		identity.SeriesID != 0 ||
		identity.EpisodeID != 0 ||
		identity.ArtistID != 0 ||
		identity.AlbumID != 0
}
