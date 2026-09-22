package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
	"github.com/z354prance/overmynd/internal/services"
)

type serviceRequest struct {
	Type             models.ServiceType `json:"type"`
	Name             string             `json:"name"`
	Enabled          bool               `json:"enabled"`
	BaseURL          string             `json:"base_url"`
	Credential       string             `json:"credential,omitempty"`
	Username         string             `json:"username,omitempty"`
	Password         string             `json:"password,omitempty"`
	UpdateCredential bool               `json:"update_credential"`
}

type publicService struct {
	ID      int64              `json:"id"`
	Type    models.ServiceType `json:"type"`
	Name    string             `json:"name"`
	Enabled bool               `json:"enabled"`
}

func (a *API) listPublicServices(
	w http.ResponseWriter,
	_ *http.Request,
) {
	items, err := a.services.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	publicItems := make([]publicService, 0, len(items))
	for _, item := range items {
		publicItems = append(publicItems, publicService{
			ID:      item.ID,
			Type:    item.Type,
			Name:    item.Name,
			Enabled: item.Enabled,
		})
	}

	writeJSON(w, http.StatusOK, publicItems)
}

func (a *API) serviceTypes(
	w http.ResponseWriter,
	_ *http.Request,
) {
	writeJSON(
		w,
		http.StatusOK,
		models.SupportedServices,
	)
}

func (a *API) listServices(
	w http.ResponseWriter,
	_ *http.Request,
) {
	items, err := a.services.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if items == nil {
		items = []models.Service{}
	}

	writeJSON(w, http.StatusOK, items)
}

func (a *API) createService(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request serviceRequest

	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	input, err := serviceInput(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	service, err := a.services.Create(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusCreated, service)
}

func (a *API) getService(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := serviceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	service, err := a.services.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	writeJSON(w, http.StatusOK, service)
}

func (a *API) updateService(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := serviceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var request serviceRequest

	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	input, err := serviceInput(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	service, err := a.services.Update(
		id,
		input,
		request.UpdateCredential,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, service)
}

func (a *API) deleteService(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := serviceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := a.services.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func serviceID(r *http.Request) (int64, error) {
	value := r.PathValue("id")

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid service id")
	}

	return id, nil
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("request must contain a single JSON value")
	}

	return nil
}

func (a *API) testService(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := serviceID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result, err := a.services.TestConnection(
		id,
		a.registry,
	)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func serviceInput(request serviceRequest) (services.Input, error) {
	credential := request.Credential

	switch request.Type {
	case models.ServiceQBittorrent,
		models.ServiceNZBGet:
		if request.Username != "" || request.Password != "" {
			encoded, err := integrations.EncodeUsernamePassword(
				request.Username,
				request.Password,
			)
			if err != nil {
				return services.Input{}, err
			}

			credential = encoded
		}
	}

	return services.Input{
		Type:       request.Type,
		Name:       request.Name,
		Enabled:    request.Enabled,
		BaseURL:    request.BaseURL,
		Credential: credential,
	}, nil
}
