package services

import "encoding/json"

type RequestSettings struct {
	Enabled   bool  `json:"enabled"`
	ServiceID int64 `json:"service_id"`
	UserID    int64 `json:"user_id"`
}

func (m *Manager) RequestSettings() (RequestSettings, error) {
	var settings RequestSettings
	value, err := m.db.GetSetting("public_media_requests")
	if err != nil || value == "" {
		return settings, err
	}
	err = json.Unmarshal([]byte(value), &settings)
	return settings, err
}

func (m *Manager) SaveRequestSettings(settings RequestSettings) error {
	value, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return m.db.SetSetting("public_media_requests", string(value))
}
