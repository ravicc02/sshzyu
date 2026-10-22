package agent

import "errors"

// FailureDetail is the bounded, credential-redacted projection of a failed
// invocation. Provider response bodies and request headers remain private.
type FailureDetail struct {
	Message            string `json:"message"`
	StatusCode         int    `json:"status_code,omitempty"`
	UpstreamStatusCode int    `json:"upstream_status_code,omitempty"`
}

func FailureFor(err error) *FailureDetail {
	var failure *InvocationFailure
	if errors.As(err, &failure) {
		return failure.Detail
	}
	return nil
}
