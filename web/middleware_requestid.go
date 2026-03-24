package web

import (
	"net/http"

	"github.com/unimap-icp-hunter/project/internal/requestid"
)

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := requestid.Normalize(r.Header.Get(requestid.HeaderName))
		if rid == "" {
			rid = requestid.New()
		}

		w.Header().Set(requestid.HeaderName, rid)
		ctx := requestid.WithContext(r.Context(), rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
