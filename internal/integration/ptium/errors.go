package ptium

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrNotConfigured means an administrator has not connected a Ptium server yet.
var ErrNotConfigured = errors.New("Ptium 연동이 설정되지 않았습니다")

// APIError carries a rejection from Ptium. The message is surfaced to the
// person who asked, because "Ptium이 거절했습니다" without the reason leaves
// them with nothing to act on.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("Ptium이 %d 상태를 반환했습니다: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("Ptium이 %d 상태를 반환했습니다", e.Status)
}

// Retryable reports whether trying again could plausibly work. A rejected
// credential or a malformed request will not fix itself.
func (e *APIError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// HTTPStatus maps a Ptium failure onto the status muni should answer with, so
// a caller can tell a misconfiguration from an outage.
func HTTPStatus(err error) int {
	var apiError *APIError
	if errors.As(err, &apiError) {
		switch apiError.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return http.StatusBadGateway
		case http.StatusNotFound:
			return http.StatusNotFound
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return http.StatusBadRequest
		}
	}
	if errors.Is(err, ErrNotConfigured) {
		return http.StatusConflict
	}
	return http.StatusBadGateway
}
