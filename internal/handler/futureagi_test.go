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

package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

type fakeFutureAGIService struct {
	setFn    func(ctx context.Context, req service.FutureAGISettings) (*service.FutureAGIInfo, common.ErrorCode, error)
	getFn    func(ctx context.Context) (*service.FutureAGIInfo, common.ErrorCode, string, error)
	deleteFn func(ctx context.Context) (bool, common.ErrorCode, string, error)
}

func (f fakeFutureAGIService) SetAPIKey(ctx context.Context, req service.FutureAGISettings) (*service.FutureAGIInfo, common.ErrorCode, error) {
	if f.setFn == nil {
		return nil, common.CodeServerError, errors.New("unexpected SetAPIKey call")
	}
	return f.setFn(ctx, req)
}

func (f fakeFutureAGIService) GetAPIKey(ctx context.Context) (*service.FutureAGIInfo, common.ErrorCode, string, error) {
	if f.getFn == nil {
		return nil, common.CodeServerError, "", errors.New("unexpected GetAPIKey call")
	}
	return f.getFn(ctx)
}

func (f fakeFutureAGIService) DeleteAPIKey(ctx context.Context) (bool, common.ErrorCode, string, error) {
	if f.deleteFn == nil {
		return false, common.CodeServerError, "", errors.New("unexpected DeleteAPIKey call")
	}
	return f.deleteFn(ctx)
}

func TestFutureAGIHandler_SetAPIKey(t *testing.T) {
	var got service.FutureAGISettings
	h := NewFutureAGIHandler(fakeFutureAGIService{
		setFn: func(ctx context.Context, req service.FutureAGISettings) (*service.FutureAGIInfo, common.ErrorCode, error) {
			got = req
			return &service.FutureAGIInfo{TenantID: "tenant-1", ProjectName: req.ProjectName, Endpoint: req.Endpoint, Redact: req.Redact, APIKey: "****-key", SecretKey: "****-sec"}, common.CodeSuccess, nil
		},
	})
	body := `{"api_key":"a-key","secret_key":"a-sec","project_name":"proj","endpoint":"http://collector:4318","redact":true}`
	resp := serveLangfuse(http.MethodPost, "/api/v1/futureagi/api-key", body, h.SetAPIKey)
	if got.APIKey != "a-key" || got.SecretKey != "a-sec" || got.ProjectName != "proj" || got.Endpoint != "http://collector:4318" || !got.Redact {
		t.Fatalf("service args req=%+v", got)
	}
	payload := decode(t, resp)
	data, _ := payload["data"].(map[string]interface{})
	if payload["code"] != float64(common.CodeSuccess) || data["secret_key"] != "****-sec" || data["project_name"] != "proj" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestFutureAGIHandler_SetAPIKey_Rejected(t *testing.T) {
	h := NewFutureAGIHandler(fakeFutureAGIService{
		setFn: func(ctx context.Context, req service.FutureAGISettings) (*service.FutureAGIInfo, common.ErrorCode, error) {
			return nil, common.CodeDataError, service.ErrFutureAGIUnauthorized
		},
	})
	payload := decode(t, serveLangfuse(http.MethodPut, "/api/v1/futureagi/api-key", `{"api_key":"x","secret_key":"y","project_name":"p"}`, h.SetAPIKey))
	if payload["code"] != float64(common.CodeDataError) || payload["message"] != "invalid Future AGI keys" {
		t.Fatalf("payload=%v", payload)
	}
	bad := decode(t, serveLangfuse(http.MethodPost, "/api/v1/futureagi/api-key", `{"redact":"yes"}`, h.SetAPIKey))
	if bad["code"] != float64(common.CodeDataError) {
		t.Fatalf("bad body payload=%v", bad)
	}
}

func TestFutureAGIHandler_GetAndDelete(t *testing.T) {
	h := NewFutureAGIHandler(fakeFutureAGIService{
		getFn: func(ctx context.Context) (*service.FutureAGIInfo, common.ErrorCode, string, error) {
			return &service.FutureAGIInfo{TenantID: "tenant-1", ProjectName: "proj", SecretKey: "****abcd"}, common.CodeSuccess, "success", nil
		},
		deleteFn: func(ctx context.Context) (bool, common.ErrorCode, string, error) {
			return true, common.CodeSuccess, "", nil
		},
	})
	got := decode(t, serveLangfuse(http.MethodGet, "/api/v1/futureagi/api-key", "", h.GetAPIKey))
	data, _ := got["data"].(map[string]interface{})
	if got["code"] != float64(common.CodeSuccess) || data["secret_key"] != "****abcd" {
		t.Fatalf("get payload=%v", got)
	}
	del := decode(t, serveLangfuse(http.MethodDelete, "/api/v1/futureagi/api-key", "", h.DeleteAPIKey))
	if del["code"] != float64(common.CodeSuccess) || del["data"] != true {
		t.Fatalf("delete payload=%v", del)
	}
}

func TestFutureAGIHandler_NoRecord(t *testing.T) {
	h := NewFutureAGIHandler(fakeFutureAGIService{
		getFn: func(ctx context.Context) (*service.FutureAGIInfo, common.ErrorCode, string, error) {
			return nil, common.CodeSuccess, "No Future AGI configuration saved.", nil
		},
		deleteFn: func(ctx context.Context) (bool, common.ErrorCode, string, error) {
			return false, common.CodeSuccess, "No Future AGI configuration saved.", nil
		},
	})
	for _, resp := range []map[string]interface{}{
		decode(t, serveLangfuse(http.MethodGet, "/api/v1/futureagi/api-key", "", h.GetAPIKey)),
		decode(t, serveLangfuse(http.MethodDelete, "/api/v1/futureagi/api-key", "", h.DeleteAPIKey)),
	} {
		if resp["code"] != float64(common.CodeSuccess) || resp["data"] != nil || resp["message"] != "No Future AGI configuration saved." {
			t.Fatalf("payload=%v", resp)
		}
	}
}
