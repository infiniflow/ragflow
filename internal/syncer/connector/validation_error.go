package connector

import "errors"

// validationError wraps a raw connector validation failure as a typed connector
// error so the /test endpoint can surface the message instead of a generic
// "check logs" response.
func validationError(err error) error {
	if err == nil {
		return nil
	}
	var (
		valErr  *ConnectorValidationError
		credErr *ConnectorMissingCredentialError
		rateErr *RateLimitTriedTooManyTimesError
	)
	if errors.As(err, &valErr) || errors.As(err, &credErr) || errors.As(err, &rateErr) {
		return err
	}
	return &ConnectorValidationError{Message: err.Error()}
}
