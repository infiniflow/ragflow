package common

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRespond(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var nilMap map[string]any
	var nilSlice []string
	tests := []struct {
		name     string
		response Response
		status   int
		body     string
	}{
		{"no data", Response{}, 200, `{"code":0,"message":"success","data":null}`},
		{"object", Response{Data: map[string]any{"ok": false}}, 200, `{"code":0,"message":"success","data":{"ok":false}}`},
		{"struct", Response{Data: struct {
			ID string `json:"id"`
		}{"one"}}, 200, `{"code":0,"message":"success","data":{"id":"one"}}`},
		{"array", Response{Data: []int{0, 1}}, 200, `{"code":0,"message":"success","data":[0,1]}`},
		{"empty object", Response{Data: map[string]any{}}, 200, `{"code":0,"message":"success","data":{}}`},
		{"empty array", Response{Data: []int{}}, 200, `{"code":0,"message":"success","data":[]}`},
		{"nil map", Response{Data: nilMap}, 200, `{"code":0,"message":"success","data":null}`},
		{"nil slice", Response{Data: nilSlice}, 200, `{"code":0,"message":"success","data":null}`},
		{"business error", Response{Code: CodeBadRequest}, 200, `{"code":400,"message":"Bad request","data":null}`},
		{"HTTP error", Response{Code: CodeBadRequest, Message: "Missing name", HTTPStatus: http.StatusBadRequest}, 400, `{"code":400,"message":"Missing name","data":null}`},
		{"version", Response{Message: "v1.2.3"}, 200, `{"code":0,"message":"v1.2.3","data":null}`},
		{"pagination", Response{Data: []string{}, Total: 0}, 200, `{"code":0,"message":"success","data":[],"total":0}`},
		{"detail", Response{Code: CodeBillingStorageInsufficient, Detail: map[string]int{"limit": 100}}, 200, `{"code":2003,"message":"Billing storage insufficient","data":null,"detail":{"limit":100}}`},
		{"raw object", Response{Data: json.RawMessage(`{"name":"test"}`)}, 200, `{"code":0,"message":"success","data":{"name":"test"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			Respond(c, tt.response)
			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}
			var got, want map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.body), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("body = %s, want %s", recorder.Body.String(), tt.body)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("Content-Type = %q", got)
			}
		})
	}
}

func TestRespondRejectsScalarAndInvalidData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, data := range map[string]any{
		"string": "secret", "bool": false, "integer": 0, "float": 1.5,
		"bytes": []byte("secret"), "raw scalar": json.RawMessage(`true`),
		"invalid JSON": json.RawMessage(`{`), "unsupported": make(chan int),
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			Respond(c, Response{Data: data, Message: "private details", Total: 5})
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if got, want := recorder.Body.String(), `{"code":500,"message":"Internal server error","data":null}`; got != want {
				t.Fatalf("body = %s, want %s", got, want)
			}
		})
	}
}
