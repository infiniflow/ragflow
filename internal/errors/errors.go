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

package errors

import "fmt"

type RetCode int

const (
	RetCodeSuccess             RetCode = 0
	RetCodeNotEffective        RetCode = 10
	RetCodeExceptionError      RetCode = 100
	RetCodeArgumentError       RetCode = 101
	RetCodeDataError           RetCode = 102
	RetCodeOperatingError      RetCode = 103
	RetCodeTimeoutError        RetCode = 104
	RetCodeConnectionError     RetCode = 105
	RetCodeRunning             RetCode = 106
	RetCodeResourceExhausted   RetCode = 107
	RetCodePermissionError     RetCode = 108
	RetCodeAuthenticationError RetCode = 109
	RetCodeBadRequest          RetCode = 400
	RetCodeUnauthorized        RetCode = 401
	RetCodeForbidden           RetCode = 403
	RetCodeNotFound            RetCode = 404
	RetCodeConflict            RetCode = 409
	RetCodeServerError         RetCode = 500
)

type AppError struct {
	Code    RetCode
	Message string
}

func (e *AppError) Error() string {
	return e.Message
}

func NewAppError(code RetCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

func ErrAuthentication(message string) *AppError {
	return NewAppError(RetCodeAuthenticationError, message)
}

func ErrServer(message string) *AppError {
	return NewAppError(RetCodeServerError, message)
}

func ErrForbidden(message string) *AppError {
	return NewAppError(RetCodeForbidden, message)
}

func ErrBadRequest(message string) *AppError {
	return NewAppError(RetCodeBadRequest, message)
}

func ErrOperating(message string) *AppError {
	return NewAppError(RetCodeOperatingError, message)
}

func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

func GetAppError(err error) (*AppError, bool) {
	appErr, ok := err.(*AppError)
	return appErr, ok
}

func WrapError(code RetCode, format string, args ...interface{}) *AppError {
	return NewAppError(code, fmt.Sprintf(format, args...))
}
