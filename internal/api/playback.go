package api

import "net/http"

func (a *API) playback(
	w http.ResponseWriter,
	r *http.Request,
) {
	result, err := a.services.Playback(
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

	for index := range result.Sessions {
		session := &result.Sessions[index]
		session.PosterURL = a.posterURL(session.SourceServiceID, session.PosterURL)
	}
	writeJSON(w, http.StatusOK, result)
}
