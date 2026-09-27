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

package server

import (
	"context"
	"errors"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var serverEE *EEServer

func init() {
	serverEE = newEEServer()
}

type EEServer struct {
	tracerProvider *sdktrace.TracerProvider
}

func newEEServer() *EEServer {
	return &EEServer{}
}

func StartServer(ctx context.Context, cancel context.CancelFunc, serverName string) error {
	if serverEE == nil {
		return errors.New("server EE is nil")
	}

	return nil
}

func ShutdownServer(ctx context.Context) error {
	if serverEE == nil {
		return errors.New("server EE is nil")
	}
	return nil
}
