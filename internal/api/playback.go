package api

import (
	"net/http"
	"os"
)

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

	result.Sessions = a.access.FilterPlayback(r.Context(), result.Sessions, os.Getenv("OVERMYND_HIDDEN_EMBY_LIBRARIES"))
	w.Header().Set("Cache-Control", "no-store")
	for index := range result.Sessions {
		session := &result.Sessions[index]
		session.PosterURL = a.posterURL(session.SourceServiceID, session.PosterURL)
	}
	writeJSON(w, http.StatusOK, result)
}
