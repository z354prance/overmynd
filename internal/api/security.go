package api

import (
	"errors"
	"net/http"
	"strings"
)

// The custom header prevents simple cross-origin form requests, including from
// older browsers without Fetch Metadata. No CORS origins are allowed. Go's
// origin protection additionally rejects cross-site and sibling-origin writes.
// Reverse proxies must preserve Host; forwarded host headers are not trusted.
func protectRequests(next http.Handler) http.Handler {
	origins := http.NewCrossOriginProtection()
	origins.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, errors.New("cross-origin request rejected"))
	}))
	protected := origins.Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("X-Overmynd-Request") != "1" {
				writeError(w, http.StatusForbidden, errors.New("missing X-Overmynd-Request header"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		}
		protected.ServeHTTP(w, r)
	})
}
