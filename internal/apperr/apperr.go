// Package apperr содержит доменные ошибки с машинными кодами.
//
// Обработчики HTTP не содержат бизнес-логики: они лишь сопоставляют
// код ошибки с HTTP-статусом и формируют конверт по контракту ТЗ
// (code, message, details, request_id).
package apperr

// Error — доменная ошибка с машинным кодом.
type Error struct {
	Code    string
	Message string
	Details any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// New создаёт ошибку с произвольным кодом.
func New(code, message string, details any) *Error {
	return &Error{Code: code, Message: message, Details: details}
}

func InvalidInput(message string, details any) *Error {
	return New("INVALID_INPUT", message, details)
}

func NotFound(message string) *Error { return New("NOT_FOUND", message, nil) }

func Forbidden(message string) *Error { return New("FORBIDDEN", message, nil) }

func VersionConflict(message string) *Error {
	return New("VERSION_CONFLICT", message, nil)
}

func NeedsData(message string, questions []string) *Error {
	return New("NEEDS_DATA", message, map[string]any{"questions": questions})
}

func InvalidTransition(message string) *Error {
	return New("INVALID_TRANSITION", message, nil)
}

func IntegrationUnavailable(message string) *Error {
	return New("INTEGRATION_UNAVAILABLE", message, nil)
}

func NotImplemented(message string) *Error {
	return New("NOT_IMPLEMENTED", message, nil)
}
