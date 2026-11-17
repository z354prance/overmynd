package api

import "net/http"

func (a *API) requests(
	w http.ResponseWriter,
	r *http.Request,
) {
	result, err := a.services.Requests(
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
