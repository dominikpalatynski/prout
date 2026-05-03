package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *Server) requireOperatorBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "

		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, prefix) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "missing bearer token",
			})
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
		expected := s.cfg.Operator.BearerToken
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "invalid bearer token",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
