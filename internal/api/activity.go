package api

import "net/http"

func (a *API) activity(
	w http.ResponseWriter,
	r *http.Request,
) {
	result, err := a.services.Activity(
		r.Context(),
		a.registry,
	)
	if err != nil {
		writeError(
			w,
			http.StatusInternalServerError,
			err,
		)
		return
	}

	for i := range result.Lifecycles {
		item := &result.Lifecycles[i]
		item.PosterURL = a.posterURL(item.PosterServiceID, item.PosterURL)
	}
	for i := range result.Seasons {
		item := &result.Seasons[i]
		item.PosterURL = a.posterURL(item.PosterServiceID, item.PosterURL)
	}
	writeJSON(w, http.StatusOK, result)
}
