//go:build integration

package api

import (
	"net/http"
	"testing"
)

func TestRESTTaskCancellationRequiresAuthentication(t *testing.T) {
	for _, token := range []string{"", "invalid-api-key"} {
		config := *TestConfig
		config.APIKey = token
		result := restCall(t, &config, http.MethodPost, "/tasks/"+uniqueName("missing_task")+"/cancel", nil, nil)
		if result.Status != http.StatusUnauthorized || result.JSON["code"] != float64(401) {
			t.Fatalf("token=%q cancellation auth: status=%d body=%v", token, result.Status, result.JSON)
		}
	}
}

func TestRESTTaskCancellationMissingAndRepeatedAreIdempotent(t *testing.T) {
	taskID := uniqueName("missing_task")
	first := restOK(t, restCall(t, TestConfig, http.MethodPost, "/tasks/"+taskID+"/cancel", nil, nil))
	if first["data"] != true {
		t.Fatalf("missing-task cancellation should acknowledge the request: %v", first)
	}
	second := restOK(t, restCall(t, TestConfig, http.MethodPost, "/tasks/"+taskID+"/cancel", nil, nil))
	if second["data"] != true {
		t.Fatalf("repeated cancellation changed acknowledgement: %v", second)
	}
}

func TestRESTTaskCancellationRejectsMalformedTaskIDs(t *testing.T) {
	for _, path := range []string{"/tasks//cancel", "/tasks/cancel", "/tasks/../cancel"} {
		result := restCall(t, TestConfig, http.MethodPost, path, nil, nil)
		if result.Status == http.StatusOK && result.JSON["code"] == float64(0) {
			t.Fatalf("malformed task path was accepted: %s body=%v", path, result.JSON)
		}
	}
}

func TestRESTTaskPatchRouteIsNotPartOfGoContract(t *testing.T) {
	// The Python test exercised PATCH /tasks/{id}, but the Go router only
	// exposes POST /tasks/{session_id}/cancel. Keep this assertion explicit so
	// a future route addition is reviewed against the supported task model.
	result := restCall(t, TestConfig, http.MethodPatch, "/tasks/"+uniqueName("missing_task"), apiObject{"action": "pause"}, nil)
	if result.Status != http.StatusNotFound && result.Status != http.StatusMethodNotAllowed {
		t.Fatalf("unsupported PATCH task route returned status=%d body=%v", result.Status, result.JSON)
	}
}
