package application

// ErrorKind 表示应用层错误的分类，接口层据此映射 HTTP 状态码与提示。
type ErrorKind string

const (
	ErrorInvalidArgument ErrorKind = "invalid_argument"
	ErrorUnauthorized    ErrorKind = "unauthorized"
	ErrorForbidden       ErrorKind = "forbidden"
	ErrorNotFound        ErrorKind = "not_found"
	ErrorConflict        ErrorKind = "conflict"
)

// AppError 是应用层统一返回的错误类型，携带错误分类与面向调用方的描述信息。
type AppError struct {
	Kind    ErrorKind
	Message string
}

// Error 实现 error 接口，返回面向调用方的错误描述。
func (e *AppError) Error() string {
	return e.Message
}

func newError(kind ErrorKind, message string) error {
	return &AppError{Kind: kind, Message: message}
}
