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

	writeJSON(w, http.StatusOK, result)
}
