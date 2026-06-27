package transcription

import (
	"errors"
	"fmt"
)

type ProviderError struct {
	Operation  string
	StatusCode int
	Retryable  bool
	Err        error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "transcription provider error"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("transcription %s failed with status %d: %v", e.Operation, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("transcription %s failed: %v", e.Operation, e.Err)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsRetryable(err error) bool {
	var providerErr *ProviderError
	return errors.As(err, &providerErr) && providerErr.Retryable
}
