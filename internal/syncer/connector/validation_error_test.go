package connector

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidationErrorPreservesTypedErrors(t *testing.T) {
	wants := []error{
		&ConnectorValidationError{Message: "validation failed"},
		&ConnectorMissingCredentialError{Message: "credential missing"},
		&RateLimitTriedTooManyTimesError{Message: "rate limited"},
	}
	for _, want := range wants {
		if got := validationError(want); got != want {
			t.Fatalf("validationError(%v) = %T(%v), want same typed error", want, got, got)
		}
	}
}

func TestValidationErrorWrapsRawError(t *testing.T) {
	raw := fmt.Errorf("GitHub API returned HTTP 401: bad credentials")
	err := validationError(raw)
	var valErr *ConnectorValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("validationError(%v) = %T, want *ConnectorValidationError", raw, err)
	}
	if valErr.Message != raw.Error() {
		t.Fatalf("message = %q, want %q", valErr.Message, raw.Error())
	}
}
