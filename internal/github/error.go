package github

import (
	"errors"
	"fmt"
)

var (
	ErrMissingSignature = errors.New("missing X-Hub-Signature-256")
	ErrInvalidSignature = errors.New("invalid GitHub webhook signature")
)

type APIError struct {
	Method     string
	URL        string
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	request := "github request failed"
	switch {
	case e.Method != "" && e.URL != "":
		request = fmt.Sprintf("github request %s %s failed", e.Method, e.URL)
	case e.Method != "":
		request = fmt.Sprintf("github request %s failed", e.Method)
	case e.URL != "":
		request = fmt.Sprintf("github request %s failed", e.URL)
	}

	if e.StatusCode <= 0 {
		if e.Message == "" {
			return request
		}
		return fmt.Sprintf("%s: %s", request, e.Message)
	}
	if e.Message == "" {
		return fmt.Sprintf("%s with status %d", request, e.StatusCode)
	}
	return fmt.Sprintf("%s with status %d: %s", request, e.StatusCode, e.Message)
}
