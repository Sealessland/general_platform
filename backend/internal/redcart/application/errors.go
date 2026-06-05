package application

type ErrorKind string

const (
	ErrorInvalidArgument ErrorKind = "invalid_argument"
	ErrorUnauthorized    ErrorKind = "unauthorized"
	ErrorForbidden       ErrorKind = "forbidden"
	ErrorNotFound        ErrorKind = "not_found"
	ErrorConflict        ErrorKind = "conflict"
)

type AppError struct {
	Kind    ErrorKind
	Message string
}

func (e *AppError) Error() string {
	return e.Message
}

func newError(kind ErrorKind, message string) error {
	return &AppError{Kind: kind, Message: message}
}
