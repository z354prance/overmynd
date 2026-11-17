package api

import (
	"github.com/z354prance/overmynd/internal/services"
	"net/http"
)

func (a *API) donations(w http.ResponseWriter, r *http.Request) {
	d, err := a.services.Donations()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, d)
}
func (a *API) saveDonations(w http.ResponseWriter, r *http.Request) {
	var d services.Donations
	if err := decodeJSON(r, &d); err != nil {
		writeError(w, 400, err)
		return
	}
	if err := a.services.SaveDonations(d); err != nil {
		writeError(w, 400, err)
		return
	}
	a.donations(w, r)
}
