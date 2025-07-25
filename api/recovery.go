package api

import (
	"fmt"
	"net/http"
)

type recoveryHandler struct {
	handler http.Handler
}

func RecoveryHandler() func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return recoveryHandler{handler: h}
	}
}

func (h recoveryHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("%v", err))
		}
	}()

	h.handler.ServeHTTP(w, req)
}
