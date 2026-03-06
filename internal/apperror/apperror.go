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

package apperror

import (
	"fmt"

	"ragflow/internal/logger"

	"go.uber.org/zap"
)

type ReturnCode int

const (
	CodeSuccess             ReturnCode = 0
	CodeNotEffective        ReturnCode = 10
	CodeExceptionError      ReturnCode = 100
	CodeArgumentError       ReturnCode = 101
	CodeDataError           ReturnCode = 102
	CodeOperatingError      ReturnCode = 103
	CodeTimeoutError        ReturnCode = 104
	CodeConnectionError     ReturnCode = 105
	CodeRunning             ReturnCode = 106
	CodeResourceExhausted   ReturnCode = 107
	CodePermissionError     ReturnCode = 108
	CodeAuthenticationError ReturnCode = 109
	CodeBadRequest          ReturnCode = 400
	CodeUnauthorized        ReturnCode = 401
	CodeForbidden           ReturnCode = 403
	CodeNotFound            ReturnCode = 404
	CodeConflict            ReturnCode = 409
	CodeServerError         ReturnCode = 500
)

type AppError struct {
	Code    ReturnCode
	Message string
}

func (e *AppError) Error() string {
	return e.Message
}

func New(code ReturnCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

func ErrAuthentication(message string) *AppError {
	return New(CodeAuthenticationError, message)
}

func ErrServer(message string) *AppError {
	return New(CodeServerError, message)
}

func ErrForbidden(message string) *AppError {
	return New(CodeForbidden, message)
}

func ErrBadRequest(message string) *AppError {
	return New(CodeBadRequest, message)
}

func ErrOperating(message string) *AppError {
	return New(CodeOperatingError, message)
}

func ErrNotFound(message string) *AppError {
	return New(CodeNotFound, message)
}

func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

func GetAppError(err error) (*AppError, bool) {
	logger.Info("GetAppError, err", zap.Error(err))
	appErr, ok := err.(*AppError)
	return appErr, ok
}

func Wrap(code ReturnCode, format string, args ...interface{}) *AppError {
	return New(code, fmt.Sprintf(format, args...))
}
