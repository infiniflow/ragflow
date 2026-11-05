//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package common

import "testing"

// allErrorCodes lists every ErrorCode constant. Keeping it in sync is what
// makes the coverage assertions below meaningful: a new code has to be added
// here together with its errorMessages entry.
var allErrorCodes = []ErrorCode{
	CodeSuccess,
	CodeLackResources,
	CodeNotEffective,
	CodeExceptionError,
	CodeArgumentError,
	CodeDataError,
	CodeOperatingError,
	CodeTimeoutError,
	CodeConnectionError,
	CodeRunning,
	CodeResourceExhausted,
	CodePermissionError,
	CodeAuthenticationError,
	CodeParamError,
	CodeConnectionPoolExhausted,
	CodeLicenseValid,
	CodeLicenseInactiveError,
	CodeLicenseExpiredError,
	CodeLicenseDigestError,
	CodeLicenseTimeRollback,
	CodeLicenseNotFound,
	CodeLicenseUnexpectedError,
	CodeLicenseNotValidYet,
	CodeErrorServerStatus,
	CodeBadRequest,
	CodeUnauthorized,
	CodeForbidden,
	CodeNotFound,
	CodeConflict,
	CodeServerError,
	CodeNotImplemented,
	CodeBillingResourceInsufficient,
	CodeBillingAppsInsufficient,
	CodeBillingSeatsInsufficient,
	CodeBillingStorageInsufficient,
	CodeBillingPointsInsufficient,
	CodeBillingSubscriptionInvalid,
	CodeBillingPaymentFailed,
}

// TestErrorMessagesCoverEveryCode keeps errorMessages one-to-one with the
// ErrorCode constants: every code has a message, no message is orphaned, and
// no code appears twice.
func TestErrorMessagesCoverEveryCode(t *testing.T) {
	listed := make(map[ErrorCode]bool, len(allErrorCodes))
	for _, code := range allErrorCodes {
		if listed[code] {
			t.Errorf("ErrorCode %d is listed twice in allErrorCodes", int(code))
			continue
		}
		listed[code] = true

		msg, ok := errorMessages[code]
		if !ok {
			t.Errorf("ErrorCode %d has no errorMessages entry", int(code))
			continue
		}
		if msg == "" {
			t.Errorf("ErrorCode %d has an empty message", int(code))
		}
	}

	if len(listed) != len(errorMessages) {
		t.Errorf("allErrorCodes lists %d distinct codes, errorMessages has %d entries", len(listed), len(errorMessages))
	}
	for code := range errorMessages {
		if !listed[code] {
			t.Errorf("errorMessages has an entry for %d but allErrorCodes does not list it", int(code))
		}
	}
}
