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

type ErrorCode int

const (
	CodeSuccess                ErrorCode = 0
	CodeNotEffective           ErrorCode = 10
	CodeExceptionError         ErrorCode = 100
	CodeArgumentError          ErrorCode = 101
	CodeDataError              ErrorCode = 102
	CodeOperatingError         ErrorCode = 103
	CodeTimeoutError           ErrorCode = 104
	CodeConnectionError        ErrorCode = 105
	CodeRunning                ErrorCode = 106
	CodeResourceExhausted      ErrorCode = 107
	CodePermissionError        ErrorCode = 108
	CodeAuthenticationError    ErrorCode = 109
	CodeParamError             ErrorCode = 110
	CodeLicenseValid           ErrorCode = 320
	CodeLicenseInactiveError   ErrorCode = 321
	CodeLicenseExpiredError    ErrorCode = 322
	CodeLicenseDigestError     ErrorCode = 323
	CodeLicenseTimeRollback    ErrorCode = 324
	CodeLicenseNotFound        ErrorCode = 325
	CodeLicenseUnexpectedError ErrorCode = 326
	CodeBadRequest             ErrorCode = 400
	CodeUnauthorized           ErrorCode = 401
	CodeForbidden              ErrorCode = 403
	CodeNotFound               ErrorCode = 404
	CodeConflict               ErrorCode = 409
	CodeServerError            ErrorCode = 500
)

var errorMessages = map[ErrorCode]string{
	CodeSuccess:                "Success",
	CodeNotEffective:           "Not effective",
	CodeExceptionError:         "System exception",
	CodeArgumentError:          "Invalid argument",
	CodeDataError:              "Data error",
	CodeOperatingError:         "Operation error",
	CodeTimeoutError:           "Timeout",
	CodeConnectionError:        "Connection error",
	CodeRunning:                "System running",
	CodeResourceExhausted:      "Resource exhausted",
	CodePermissionError:        "Permission denied",
	CodeAuthenticationError:    "Authentication failed",
	CodeParamError:             "Invalid parameters",
	CodeLicenseValid:           "License valid",
	CodeLicenseInactiveError:   "License inactive",
	CodeLicenseExpiredError:    "License expired",
	CodeLicenseDigestError:     "License digest error",
	CodeLicenseTimeRollback:    "License time rollback detected",
	CodeLicenseNotFound:        "License not found",
	CodeLicenseUnexpectedError: "Unexpected license error",
	CodeBadRequest:             "Bad request",
	CodeUnauthorized:           "Unauthorized",
	CodeForbidden:              "Forbidden",
	CodeNotFound:               "Resource not found",
	CodeConflict:               "Resource conflict",
	CodeServerError:            "Internal server error",
}

func (e ErrorCode) Message() string {
	if msg, ok := errorMessages[e]; ok {
		return msg
	}
	return "Unknown error"
}
