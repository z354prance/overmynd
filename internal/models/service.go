package models

import "time"

type ServiceType string

const (
	ServiceTracearr    ServiceType = "tracearr"
	ServiceRadarr      ServiceType = "radarr"
	ServiceSonarr      ServiceType = "sonarr"
	ServiceLidarr      ServiceType = "lidarr"
	ServiceSeerr       ServiceType = "seerr"
	ServiceTdarr       ServiceType = "tdarr"
	ServiceQBittorrent ServiceType = "qbittorrent"
	ServiceNZBGet      ServiceType = "nzbget"
)

var SupportedServices = []ServiceType{
	ServiceTracearr,
	ServiceRadarr,
	ServiceSonarr,
	ServiceLidarr,
	ServiceSeerr,
	ServiceTdarr,
	ServiceQBittorrent,
	ServiceNZBGet,
}

type Service struct {
	ID            int64       `json:"id"`
	Type          ServiceType `json:"type"`
	Name          string      `json:"name"`
	Enabled       bool        `json:"enabled"`
	BaseURL       string      `json:"base_url"`
	HasCredential bool        `json:"has_credential"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

type ServiceHealth struct {
	ServiceID     int64      `json:"service_id"`
	Status        string     `json:"status"`
	Message       string     `json:"message,omitempty"`
	LastCheckAt   *time.Time `json:"last_check_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
}

type ServiceStatus struct {
	Service Service        `json:"service"`
	Health  *ServiceHealth `json:"health,omitempty"`
}
