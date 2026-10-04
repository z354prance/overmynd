package api

import "net/http"

func (a *API) recentlyAdded(w http.ResponseWriter, r *http.Request) {
	result, err := a.services.RecentlyAdded(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for i := range result.Items {
		item := &result.Items[i]
		item.PosterURL = a.posterURL(item.PosterServiceID, item.PosterURL)
	}
	writeJSON(w, http.StatusOK, result)
}
