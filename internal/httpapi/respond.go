package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"oma/internal/apperr"
)

// codeStatus сопоставляет машинный код ошибки с HTTP-статусом.
// Список кодов — раздел 7.1 технической спецификации.
var codeStatus = map[string]int{
	"INVALID_INPUT":           http.StatusBadRequest,
	"NOT_FOUND":               http.StatusNotFound,
	"FORBIDDEN":               http.StatusForbidden,
	"VERSION_CONFLICT":        http.StatusConflict,
	"INVALID_TRANSITION":      http.StatusConflict,
	"REPORT_CHANGED":          http.StatusConflict,
	"CART_CHANGED":            http.StatusConflict,
	"NEEDS_DATA":              http.StatusUnprocessableEntity,
	"REVIEW_REQUIRED":         http.StatusUnprocessableEntity,
	"REVIEW_STALE":            http.StatusUnprocessableEntity,
	"QUOTE_EXPIRED":           http.StatusConflict,
	"CHECKOUT_DECLINED":       http.StatusConflict,
	"INTEGRATION_UNAVAILABLE": http.StatusServiceUnavailable,
	"NOT_IMPLEMENTED":         http.StatusNotImplemented,
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("httpapi: ошибка записи ответа: %v", err)
	}
}

// respondError формирует конверт ошибки {code, message, details, request_id}.
func respondError(w http.ResponseWriter, r *http.Request, err error) {
	rid := requestIDFrom(r.Context())
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		log.Printf("httpapi: внутренняя ошибка: %v", err)
		ae = &apperr.Error{Code: "INTERNAL", Message: "внутренняя ошибка сервера"}
	}
	status, ok := codeStatus[ae.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	if ae.Details == nil {
		ae.Details = map[string]any{}
	}
	writeJSON(w, status, map[string]any{
		"code":       ae.Code,
		"message":    ae.Message,
		"details":    ae.Details,
		"request_id": rid,
	})
}

// decodeJSON разбирает тело запроса; ошибка разбора — INVALID_INPUT.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		respondError(w, r, apperr.InvalidInput("некорректное тело запроса: "+err.Error(), nil))
		return false
	}
	return true
}
