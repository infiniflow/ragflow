package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestReplicateSucceededPredictionStopsPolling(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(replicatePrediction{Status: "succeeded", Output: []float64{0.25, 0.5}})
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
	defer cancel()
	model := newReplicateForTest(srv.URL)
	prediction, err := model.waitForPrediction(ctx, &replicatePrediction{Status: "processing", URLs: replicatePredictionURLs{Get: srv.URL}}, "test-key")
	if err != nil {
		t.Fatalf("completed prediction should return output: %v", err)
	}
	if prediction.Status != "succeeded" {
		t.Fatalf("status=%q", prediction.Status)
	}
}

func TestReplicateEmbedReturnsSucceededPrediction(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(replicatePrediction{Status: "succeeded", Output: [][]float64{{0.25, 0.5}}})
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	name, key := "owner/model", "test-key"
	vectors, err := newReplicateForTest(srv.URL).Embed(ctx, &name, EmbedRequest{Texts: []string{"title"}}, &APIConfig{ApiKey: &key}, nil, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != 1 || !reflect.DeepEqual(vectors[0].Embedding, []float64{0.25, 0.5}) {
		t.Fatalf("vectors=%v", vectors)
	}
}
