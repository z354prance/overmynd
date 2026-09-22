package nzbget

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type group struct {
	NZBID             int64  `json:"NZBID"`
	NZBName           string `json:"NZBName"`
	Filename          string `json:"Filename"`
	DestDir           string `json:"DestDir"`
	Status            string `json:"Status"`
	FileSizeMB        int64  `json:"FileSizeMB"`
	FileSizeLo        int64  `json:"FileSizeLo"`
	FileSizeHi        int64  `json:"FileSizeHi"`
	RemainingSizeMB   int64  `json:"RemainingSizeMB"`
	RemainingSizeLo   int64  `json:"RemainingSizeLo"`
	RemainingSizeHi   int64  `json:"RemainingSizeHi"`
	MinPostTime       int64  `json:"MinPostTime"`
	MaxPostTime       int64  `json:"MaxPostTime"`
	ActiveDownloads   int64  `json:"ActiveDownloads"`
	PostInfoText      string `json:"PostInfoText"`
	RemainingParCount int64  `json:"RemainingParCount"`
}

func (i *Integration) Queue(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.Download, error) {
	auth, err := integrations.DecodeUsernamePassword(credential)
	if err != nil {
		return nil, err
	}

	client := NewClient()

	var groups []group
	if err := client.Call(
		ctx,
		service.BaseURL,
		auth.Username,
		auth.Password,
		"listgroups",
		&groups,
	); err != nil {
		return nil, err
	}

	items := make([]models.Download, 0, len(groups))

	for _, group := range groups {
		if isCompletedGroup(group) {
			continue
		}

		title := strings.TrimSpace(group.NZBName)
		if title == "" {
			title = strings.TrimSpace(group.Filename)
		}
		if title == "" {
			title = fmt.Sprintf("NZB %d", group.NZBID)
		}

		size := nzbSize(group.FileSizeHi, group.FileSizeLo)
		sizeLeft := nzbSize(
			group.RemainingSizeHi,
			group.RemainingSizeLo,
		)

		if size == 0 && group.FileSizeMB > 0 {
			size = group.FileSizeMB * 1024 * 1024
		}
		if sizeLeft == 0 && group.RemainingSizeMB > 0 {
			sizeLeft = group.RemainingSizeMB * 1024 * 1024
		}

		item := models.Download{
			ID:              strconv.FormatInt(service.ID, 10) + ":nzb:" + strconv.FormatInt(group.NZBID, 10),
			Source:          service.Type,
			SourceServiceID: service.ID,
			Title:           title,
			Status:          group.Status,
			Protocol:        "usenet",
			DownloadClient:  service.Name,
			DownloadID:      strconv.FormatInt(group.NZBID, 10),
			OutputPath:      group.DestDir,
			Size:            size,
			SizeLeft:        sizeLeft,
		}

		items = append(items, item)
	}

	return items, nil
}

func nzbSize(hi int64, lo int64) int64 {
	return hi<<32 + lo
}

func isCompletedGroup(group group) bool {
	status := strings.ToUpper(strings.TrimSpace(group.Status))

	switch status {
	case "SUCCESS", "DELETED":
		return true
	default:
		return false
	}
}

func unixTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}

	result := time.Unix(value, 0).UTC()
	return &result
}
