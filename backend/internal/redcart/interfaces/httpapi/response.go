package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

func decodeJSON(r *http.Request, out any) error {
	if r.Body == nil {
		return errEmptyBody
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		if err.Error() == "EOF" {
			return errEmptyBody
		}
		return err
	}
	return nil
}

var errEmptyBody = fmt.Errorf("empty request body")

func parseIDFromPath(path, prefix string) (int64, error) {
	idStr := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if strings.Contains(idStr, "/") {
		return 0, fmt.Errorf("invalid path")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func parseSuffixID(path, suffix string) (int64, error) {
	idStr := strings.TrimSuffix(path, suffix)
	idStr = strings.Trim(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
}

func writeBadRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"kind":    "bad_request",
			"message": err.Error(),
		},
	})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"error": map[string]any{
			"kind":    "method_not_allowed",
			"message": "method not allowed",
		},
	})
}

func writeAppError(w http.ResponseWriter, err error) {
	appErr, ok := err.(*application.AppError)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{
				"kind":    "internal",
				"message": err.Error(),
			},
		})
		return
	}
	status := http.StatusBadRequest
	switch appErr.Kind {
	case application.ErrorUnauthorized:
		status = http.StatusUnauthorized
	case application.ErrorForbidden:
		status = http.StatusForbidden
	case application.ErrorNotFound:
		status = http.StatusNotFound
	case application.ErrorConflict:
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"kind":    string(appErr.Kind),
			"message": appErr.Message,
		},
	})
}
