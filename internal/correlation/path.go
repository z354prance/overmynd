package correlation

import (
	"path"
	"strings"

	"github.com/z354prance/overmynd/internal/models"
)

func MatchDownloadProcessingPath(
	download models.Download,
	job models.ProcessingJob,
) Match {
	a := pathParts(download.OutputPath)
	b := pathParts(job.Path)

	if len(a) == 0 || len(b) == 0 {
		return Match{}
	}

	common := commonPathTail(a, b)

	if common >= 2 {
		return Match{
			Matched:  true,
			Reason:   models.CorrelationPath,
			Strength: models.CorrelationMedium,
			Key:      "path:" + strings.Join(a[len(a)-common:], "/"),
		}
	}

	// A filename-only match is useful evidence, but deliberately weak.
	if common == 1 {
		return Match{
			Matched:  true,
			Reason:   models.CorrelationPath,
			Strength: models.CorrelationWeak,
			Key:      "filename:" + a[len(a)-1],
		}
	}

	return Match{}
}

func pathParts(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	// Normalize Windows-style separators as well as Unix paths.
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Clean(value)

	raw := strings.Split(value, "/")
	parts := make([]string, 0, len(raw))

	for _, part := range raw {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" || part == "." {
			continue
		}
		parts = append(parts, part)
	}

	return parts
}

func commonPathTail(a, b []string) int {
	i := len(a) - 1
	j := len(b) - 1
	count := 0

	for i >= 0 && j >= 0 {
		if a[i] != b[j] {
			break
		}

		count++
		i--
		j--
	}

	return count
}
