package api

import "net/http"

func (a *API) processing(
	w http.ResponseWriter,
	r *http.Request,
) {
	result, err := a.services.Processing(
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
