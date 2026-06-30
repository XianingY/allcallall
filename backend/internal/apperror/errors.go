package apperror

import (
	"fmt"
	"net/http"
)

type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Internal   error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Internal
}

func New(code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

func Wrap(err error, code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Internal:   err,
	}
}

var (
	ErrCodeNotFound             = "NOT_FOUND"
	ErrCodeInvalidRequest       = "INVALID_REQUEST"
	ErrCodeInternalServerError  = "INTERNAL_SERVER_ERROR"
	ErrCodeUnauthorized         = "UNAUTHORIZED"
	ErrCodeForbidden            = "FORBIDDEN"
	ErrCodeInsufficientBalance  = "INSUFFICIENT_BALANCE"
	ErrCodeRateLimited          = "RATE_LIMITED"
)

func NewNotFound(message string) *AppError {
	return New(ErrCodeNotFound, message, http.StatusNotFound)
}

func NewInvalidRequest(message string) *AppError {
	return New(ErrCodeInvalidRequest, message, http.StatusBadRequest)
}

func NewInternal(err error, message string) *AppError {
	return Wrap(err, ErrCodeInternalServerError, message, http.StatusInternalServerError)
}

func NewUnauthorized(message string) *AppError {
	return New(ErrCodeUnauthorized, message, http.StatusUnauthorized)
}

func NewForbidden(message string) *AppError {
	return New(ErrCodeForbidden, message, http.StatusForbidden)
}

func NewInsufficientBalance(message string) *AppError {
	return New(ErrCodeInsufficientBalance, message, http.StatusPaymentRequired)
}

func NewRateLimited(message string) *AppError {
	return New(ErrCodeRateLimited, message, http.StatusTooManyRequests)
}
