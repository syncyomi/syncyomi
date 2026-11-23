package http

import (
	"net/http"
	"slices"

	"github.com/rs/cors"
)

var corsMethods = []string{"HEAD", "OPTIONS", "GET", "POST", "PUT", "PATCH", "DELETE"}

func newCORS(allowedOrigins []string) func(http.Handler) http.Handler {
	isListed := func(origin string) bool { return slices.Contains(allowedOrigins, origin) }

	credentialed := cors.New(cors.Options{
		AllowOriginFunc:    isListed,
		AllowCredentials:   true,
		AllowedMethods:     corsMethods,
		OptionsPassthrough: true,
	})
	anonymous := cors.New(cors.Options{
		AllowedOrigins:     []string{"*"},
		AllowedMethods:     corsMethods,
		OptionsPassthrough: true,
	})

	return func(next http.Handler) http.Handler {
		withCredentials, withoutCredentials := credentialed.Handler(next), anonymous.Handler(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isListed(r.Header.Get("Origin")) {
				withCredentials.ServeHTTP(w, r)
				return
			}

			withoutCredentials.ServeHTTP(w, r)
		})
	}
}
