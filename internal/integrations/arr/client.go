package arr

import (
	"context"
	"fmt"
	"strings"

	"github.com/z354prance/overmynd/internal/integrations"
)

type Client struct {
	http       *integrations.HTTPClient
	apiVersion string
}

type SystemStatus struct {
	AppName           string `json:"appName"`
	InstanceName      string `json:"instanceName"`
	Version           string `json:"version"`
	BuildTime         string `json:"buildTime"`
	IsDebug           bool   `json:"isDebug"`
	IsProduction      bool   `json:"isProduction"`
	IsAdmin           bool   `json:"isAdmin"`
	IsUserInteractive bool   `json:"isUserInteractive"`
	StartupPath       string `json:"startupPath"`
	AppData           string `json:"appData"`
	OSName            string `json:"osName"`
	OSVersion         string `json:"osVersion"`
}

func NewClient(apiVersion string) *Client {
	apiVersion = strings.TrimSpace(apiVersion)

	return &Client{
		http:       integrations.NewHTTPClient(),
		apiVersion: apiVersion,
	}
}

func (c *Client) Endpoint(
	baseURL string,
	path string,
) (string, error) {
	baseURL, err := integrations.NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}

	if c.apiVersion == "" {
		return "", fmt.Errorf("ARR API version is required")
	}

	path = strings.TrimLeft(path, "/")

	return fmt.Sprintf(
		"%s/api/%s/%s",
		baseURL,
		c.apiVersion,
		path,
	), nil
}

func (c *Client) SystemStatus(
	ctx context.Context,
	baseURL string,
	apiKey string,
) (SystemStatus, error) {
	if apiKey == "" {
		return SystemStatus{}, fmt.Errorf("API key is required")
	}

	endpoint, err := c.Endpoint(
		baseURL,
		"system/status",
	)
	if err != nil {
		return SystemStatus{}, err
	}

	var status SystemStatus

	err = c.http.GetJSON(
		ctx,
		endpoint,
		map[string]string{
			"X-Api-Key": apiKey,
		},
		&status,
	)
	if err != nil {
		return SystemStatus{}, err
	}

	if status.Version == "" {
		return SystemStatus{}, fmt.Errorf(
			"service returned no version",
		)
	}

	return status, nil
}
