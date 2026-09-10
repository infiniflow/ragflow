// tests/api/user_api_test.go
////go:build integration

package api

import (
	"testing"
)

func TestCreateDataset(t *testing.T) {
	if TestPriority >= 1 {
		t.Log("Skip TestCreateDataset")
	}
	//resp, err := http.Post(baseURL+"/users", "application/json", body)
	//assert.NoError(t, err)
	//assert.Equal(t, 201, resp.StatusCode)
	t.Log("TestCreateDataset", TestPriority)
}
