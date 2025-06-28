package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

func readBodyAsJSON[requestSchema any](r *http.Request) (*requestSchema, error) {
	if r.Method == http.MethodConnect {
		return nil, errors.New("GET method")
	}

	if r.Body == nil {
		return nil, errors.New("empty body")
	}

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		return nil, fmt.Errorf("incorrect content type: %s", contentType)
	}

	var req requestSchema
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.New("body is not valid JSON")
	}

	return &req, nil
}

func writeResponseAsJSON(w http.ResponseWriter, response any, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "couldn't write response", http.StatusInternalServerError)
	}
}

type apiErrorCode string

const (
	codeInvalidRequest   apiErrorCode = "INVALID_REQUEST"
	codeInvalidParameter apiErrorCode = "INVALID_PARAMETER"

	codeUnauthenticated apiErrorCode = "UNAUTHENTICATED"

	codeMustBeOwner apiErrorCode = "MUST_BE_OWNER"

	codeInternalError apiErrorCode = "INTERNAL_ERROR"
)

func writeError(w http.ResponseWriter, apiCode apiErrorCode, message string) {
	errorResponse := struct {
		Message string       `json:"message"`
		Code    apiErrorCode `json:"code"`
	}{
		Message: message,
		Code:    apiCode,
	}

	writeResponseAsJSON(w, errorResponse, apiCodeToHTTPStatusCode(apiCode))
}

func apiCodeToHTTPStatusCode(code apiErrorCode) int {
	switch code {
	case codeUnauthenticated:
		return 401
	case codeMustBeOwner:
		return 403
	case codeInternalError:
		return 500
	default:
		return 400
	}
}
