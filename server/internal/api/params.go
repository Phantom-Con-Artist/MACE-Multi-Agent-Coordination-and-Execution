package api

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"
)

// Chi preserves escaped path segments. Browsers encode composite peer IDs.
func routeParam(r *http.Request, key string) string {
	raw := chi.URLParam(r, key)
	value, e := url.PathUnescape(raw)
	if e != nil {
		return raw
	}
	return value
}
