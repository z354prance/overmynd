package correlation

import (
	"regexp"
	"strings"

	"github.com/z354prance/overmynd/internal/models"
)

var metadataSeparators = regexp.MustCompile(`[\s._-]+`)

type MetadataIdentity struct {
	Kind models.MediaKind

	Title string
	Year  int

	SeasonNumber  int
	EpisodeNumber int
}

func MetadataFromAttention(item models.AttentionItem) MetadataIdentity {
	return MetadataIdentity{
		Kind:          item.Kind,
		Title:         normalizeTitle(item.Title),
		Year:          item.Year,
		SeasonNumber:  item.SeasonNumber,
		EpisodeNumber: item.EpisodeNumber,
	}
}

func MetadataFromDownload(item models.Download) MetadataIdentity {
	return MetadataIdentity{
		Title: normalizeTitle(item.Title),
	}
}

func MetadataFromProcessing(item models.ProcessingJob) MetadataIdentity {
	return MetadataIdentity{
		Title: normalizeTitle(item.Title),
	}
}

func MetadataFromPlayback(item models.PlaybackSession) MetadataIdentity {
	kind := mediaKindFromPlayback(item.MediaType)

	title := item.MediaTitle
	if kind == models.MediaEpisode && item.ShowTitle != "" {
		title = item.ShowTitle
	}

	return MetadataIdentity{
		Kind:          kind,
		Title:         normalizeTitle(title),
		Year:          item.Year,
		SeasonNumber:  item.SeasonNumber,
		EpisodeNumber: item.EpisodeNumber,
	}
}

func MatchMetadata(a, b MetadataIdentity) Match {
	if a.Title == "" || b.Title == "" || a.Title != b.Title {
		return Match{}
	}

	if a.Kind != "" && b.Kind != "" && a.Kind != b.Kind {
		return Match{}
	}

	if a.Kind == models.MediaEpisode || b.Kind == models.MediaEpisode {
		if a.SeasonNumber == 0 ||
			b.SeasonNumber == 0 ||
			a.EpisodeNumber == 0 ||
			b.EpisodeNumber == 0 {
			return Match{}
		}

		if a.SeasonNumber != b.SeasonNumber ||
			a.EpisodeNumber != b.EpisodeNumber {
			return Match{}
		}

		return Match{
			Matched:  true,
			Reason:   models.CorrelationMetadata,
			Strength: models.CorrelationWeak,
			Key:      metadataKey(a),
		}
	}

	if a.Year != 0 && b.Year != 0 && a.Year != b.Year {
		return Match{}
	}

	return Match{
		Matched:  true,
		Reason:   models.CorrelationMetadata,
		Strength: models.CorrelationWeak,
		Key:      metadataKey(a),
	}
}

func metadataKey(item MetadataIdentity) string {
	var builder strings.Builder

	builder.WriteString("metadata:")
	builder.WriteString(string(item.Kind))
	builder.WriteString(":")
	builder.WriteString(item.Title)

	if item.Year != 0 {
		builder.WriteString(":")
		builder.WriteString(intString(item.Year))
	}

	if item.SeasonNumber != 0 || item.EpisodeNumber != 0 {
		builder.WriteString(":s")
		builder.WriteString(intString(item.SeasonNumber))
		builder.WriteString("e")
		builder.WriteString(intString(item.EpisodeNumber))
	}

	return builder.String()
}

func normalizeTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = metadataSeparators.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
}

func intString(value int) string {
	if value == 0 {
		return "0"
	}

	const digits = "0123456789"

	var buffer [20]byte
	pos := len(buffer)

	for value > 0 {
		pos--
		buffer[pos] = digits[value%10]
		value /= 10
	}

	return string(buffer[pos:])
}
