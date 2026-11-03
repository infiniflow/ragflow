package common

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestResponseHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var nilMap map[string]any
	var nilSlice []string
	var nilPointer *struct{ ID string }
	objectPointer := &struct {
		ID string `json:"id"`
	}{"one"}
	objectMap := map[string]string{"id": "one"}
	list := []int{0, 1}
	scalar := "scalar"
	tests := []struct {
		name   string
		write  func(*gin.Context)
		status int
		body   string
	}{
		{"no data", func(c *gin.Context) { RespondSuccess(c, nil, "") }, 200, `{"code":0,"message":"success"}`},
		{"object", func(c *gin.Context) { RespondSuccess(c, map[string]any{"ok": false}, "") }, 200, `{"code":0,"message":"success","data":{"ok":false}}`},
		{"struct", func(c *gin.Context) {
			RespondSuccess(c, struct {
				ID string `json:"id"`
			}{"one"}, "")
		}, 200, `{"code":0,"message":"success","data":{"id":"one"}}`},
		{"list", func(c *gin.Context) { RespondSuccessList(c, []int{0, 1}, "") }, 200, `{"code":0,"message":"success","data":[0,1]}`},
		{"array", func(c *gin.Context) { RespondSuccess(c, [2]int{0, 1}, "") }, 200, `{"code":0,"message":"success","data":[0,1]}`},
		{"object pointer", func(c *gin.Context) { RespondSuccess(c, objectPointer, "") }, 200, `{"code":0,"message":"success","data":{"id":"one"}}`},
		{"nested object pointer", func(c *gin.Context) { RespondSuccess(c, &objectPointer, "") }, 200, `{"code":0,"message":"success","data":{"id":"one"}}`},
		{"typed map", func(c *gin.Context) { RespondSuccess(c, objectMap, "") }, 200, `{"code":0,"message":"success","data":{"id":"one"}}`},
		{"map pointer", func(c *gin.Context) { RespondSuccess(c, &objectMap, "") }, 200, `{"code":0,"message":"success","data":{"id":"one"}}`},
		{"slice pointer", func(c *gin.Context) { RespondSuccess(c, &list, "") }, 200, `{"code":0,"message":"success","data":[0,1]}`},
		{"empty object", func(c *gin.Context) { RespondSuccess(c, map[string]any{}, "") }, 200, `{"code":0,"message":"success","data":{}}`},
		{"empty list", func(c *gin.Context) { RespondSuccessList(c, []int{}, "") }, 200, `{"code":0,"message":"success","data":[]}`},
		{"nil object", func(c *gin.Context) { RespondSuccess(c, nilMap, "") }, 200, `{"code":0,"message":"success"}`},
		{"nil pointer", func(c *gin.Context) { RespondSuccess(c, nilPointer, "") }, 200, `{"code":0,"message":"success"}`},
		{"nil list", func(c *gin.Context) { RespondSuccessList(c, nilSlice, "") }, 200, `{"code":0,"message":"success"}`},
		{"pointer to nil map", func(c *gin.Context) { RespondSuccess(c, &nilMap, "") }, 200, `{"code":0,"message":"success"}`},
		{"pointer to nil list", func(c *gin.Context) { RespondSuccess(c, &nilSlice, "") }, 200, `{"code":0,"message":"success"}`},
		{"business error", func(c *gin.Context) { RespondError(c, CodeBadRequest, "") }, 200, `{"code":400,"message":"Bad request"}`},
		{"zero HTTP status", func(c *gin.Context) { Respond(c, 0, CodeSuccess, nil, "") }, 200, `{"code":0,"message":"success"}`},
		{"HTTP error", func(c *gin.Context) {
			Respond(c, http.StatusBadRequest, CodeArgumentError, nil, "Missing name")
		}, 400, `{"code":101,"message":"Missing name"}`},
		{"string", func(c *gin.Context) { RespondSuccess(c, "v1.2.3", "") }, 200, `{"code":0,"message":"success"}`},
		{"empty string", func(c *gin.Context) { RespondSuccess(c, "", "") }, 200, `{"code":0,"message":"success"}`},
		{"boolean", func(c *gin.Context) { RespondSuccess(c, false, "") }, 200, `{"code":0,"message":"success"}`},
		{"integer", func(c *gin.Context) { RespondSuccess(c, int64(0), "") }, 200, `{"code":0,"message":"success"}`},
		{"true", func(c *gin.Context) { Respond(c, http.StatusCreated, CodeSuccess, true, "") }, 201, `{"code":0,"message":"success"}`},
		{"float", func(c *gin.Context) { RespondSuccess(c, 1.5, "") }, 200, `{"code":0,"message":"success"}`},
		{"scalar pointer", func(c *gin.Context) { RespondSuccess(c, &scalar, "") }, 200, `{"code":0,"message":"success"}`},
		{"bytes retain encoding", func(c *gin.Context) { RespondSuccess(c, []byte("bytes"), "") }, 200, `{"code":0,"message":"success","data":"Ynl0ZXM="}`},
		{"struct retains custom encoding", func(c *gin.Context) { RespondSuccess(c, time.Unix(0, 0).UTC(), "") }, 200, `{"code":0,"message":"success","data":"1970-01-01T00:00:00Z"}`},
		{"raw object", func(c *gin.Context) { RespondSuccess(c, json.RawMessage(` {"id":"raw"} `), "") }, 200, `{"code":0,"message":"success","data":{"id":"raw"}}`},
		{"raw array", func(c *gin.Context) { RespondSuccess(c, json.RawMessage(`[1,"two",false]`), "") }, 200, `{"code":0,"message":"success","data":[1,"two",false]}`},
		{"raw scalar retains encoding", func(c *gin.Context) { RespondSuccess(c, json.RawMessage(`true`), "") }, 200, `{"code":0,"message":"success","data":true}`},
		{"raw null retains encoding", func(c *gin.Context) { RespondSuccess(c, json.RawMessage(`null`), "") }, 200, `{"code":0,"message":"success","data":null}`},
		{"unsupported data", func(c *gin.Context) { RespondSuccess(c, make(chan int), "") }, 200, `{"code":0,"message":"success"}`},
		{"error message", func(c *gin.Context) { RespondError(c, CodeDataError, "Missing document") }, 200, `{"code":102,"message":"Missing document"}`},
		{"pagination", func(c *gin.Context) {
			RespondSuccessList(c, []string{}, "", 0)
		}, 200, `{"code":0,"message":"success","data":[],"total":0}`},
		{"paginated list", func(c *gin.Context) {
			RespondSuccessList(c, []string{"one"}, "listed", 42)
		}, 200, `{"code":0,"message":"listed","data":["one"],"total":42}`},
		{"large total", func(c *gin.Context) {
			var total int64 = 1 << 40
			RespondSuccessList(c, []string{}, "", total)
		}, 200, `{"code":0,"message":"success","data":[],"total":1099511627776}`},
		{"nil list with total", func(c *gin.Context) {
			RespondSuccessList(c, nilSlice, "", 0)
		}, 200, `{"code":0,"message":"success","total":0}`},
		{"detail", func(c *gin.Context) {
			RespondError(c, CodeBillingStorageInsufficient, "", ResponseOptions{Detail: map[string]int{"limit": 100}})
		}, 200, `{"code":2003,"message":"Billing storage insufficient","detail":{"limit":100}}`},
		{"combined options", func(c *gin.Context) {
			Respond(c, http.StatusCreated, CodeSuccess, []string{"one"}, "created", ResponseOptions{Total: 1})
		}, 201, `{"code":0,"message":"created","data":["one"],"total":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			tt.write(c)
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
