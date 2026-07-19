// Package changerequest validates the change-request (CR) code that must
// accompany a promotion into a guarded ring (typically acceptance and
// production). A Validator checks a code against a business system — currently
// JIRA — so an operator cannot promote to prod without a real, approved change.
//
// The universal demo code "test" is NOT handled here: the promoter accepts it
// before ever calling a Validator, so demos work against any provider. A
// Validator therefore only ever sees codes that must be checked for real.
package changerequest

import (
	"context"
	"errors"
)

// Sentinel errors. The promoter wraps these into gate errors the API maps to a
// 4xx response.
var (
	// ErrCodeRequired means the gate is enabled but no code was supplied.
	ErrCodeRequired = errors.New("change request code required")
	// ErrInvalidCode means the supplied code did not validate (unknown issue,
	// wrong project, or a status that does not authorize the change).
	ErrInvalidCode = errors.New("invalid change request code")
)

// Validator validates a change-request code for a promotion of app into ring.
// It returns nil when the code authorizes the promotion, ErrInvalidCode when it
// does not, or another error if the backing system could not be reached.
type Validator interface {
	Validate(ctx context.Context, app, ring, code string) error
}

// Test is the validator used when the change-request gate is enabled but no
// business system is configured (provider: "test"). The only code it would ever
// accept is the demo code "test", which the promoter already accepts before
// calling any Validator — so Test rejects everything it is asked to check.
type Test struct{}

// Validate always rejects: any code other than the promoter-handled "test"
// demo code is invalid when no real system is wired up.
func (Test) Validate(_ context.Context, _, _, _ string) error {
	return errors.Join(ErrInvalidCode, errors.New(`no change-request system configured; only the demo code "test" is accepted`))
}
