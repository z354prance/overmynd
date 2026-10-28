package access

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Verification must not POST /Connect/Link: that creates a link and can reject
// an already linked account or send another invitation.
func verifyConnect(ctx context.Context, s Settings, r Request, user embyUser) error {
	if user.ConnectLinkType != "LinkedUser" || strings.TrimSpace(user.ConnectUserName) == "" {
		return errors.New("Emby has not confirmed your Connect link yet; confirm the linking email, then retry")
	}
	if !strings.EqualFold(strings.TrimSpace(user.ConnectUserName), strings.TrimSpace(r.Connect)) {
		return errors.New("the linked Emby Connect identity does not match the approved request; contact the server owner")
	}
	var pending []json.RawMessage
	if err := emby(ctx, s, "GET", "/Connect/Pending", nil, &pending); err != nil {
		return errors.New("unable to read pending Emby Connect links; retry or contact the server owner")
	}
	// A null/unknown response is not evidence that the pending list is empty.
	if pending == nil {
		return errors.New("Emby returned an unrecognized pending-link response; contact the server owner")
	}
	for _, raw := range pending {
		var entry struct {
			LocalUserID string `json:"LocalUserId"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil || strings.TrimSpace(entry.LocalUserID) == "" {
			// The API does not publish a response schema. Do not assume an unidentified
			// pending entry belongs to someone else and enable an unconfirmed account.
			return errors.New("Emby has pending Connect links that cannot be identified safely; contact the server owner")
		}
		if strings.EqualFold(strings.ReplaceAll(entry.LocalUserID, "-", ""), strings.ReplaceAll(r.EmbyID, "-", "")) {
			return errors.New("Emby Connect confirmation is still pending; confirm the linking email, then retry")
		}
	}
	return nil
}
