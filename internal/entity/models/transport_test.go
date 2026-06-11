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

package models

import (
	"net/http"
	"testing"
	"time"
)

// roundTripperFunc adapts a func into an http.RoundTripper for the tests.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func swapDefaultTransport(t *testing.T) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = original
	})
}

func assertPoolDefaults(t *testing.T, transport *http.Transport) {
	t.Helper()
	if transport == nil {
		t.Fatal("newPooledTransport returned nil")
	}
	if transport.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns=%d, want 100", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost=%d, want 10", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout=%s, want 90s", transport.IdleConnTimeout)
	}
}

func TestNewPooledTransportClonesStandardDefault(t *testing.T) {
	transport := newPooledTransport()
	assertPoolDefaults(t, transport)
	if transport.Proxy == nil {
		t.Error("expected Proxy to be preserved from the default transport")
	}
}

// Regression: a non-*http.Transport default must not panic the constructor.
func TestNewPooledTransportWithCustomDefaultTransport(t *testing.T) {
	swapDefaultTransport(t)
	transport := newPooledTransport()
	assertPoolDefaults(t, transport)
	if transport.Proxy == nil {
		t.Error("expected fallback transport to set Proxy")
	}
}

func TestProviderConstructorsWithCustomDefaultTransport(t *testing.T) {
	swapDefaultTransport(t)

	base := map[string]string{"default": "http://unused"}
	suffix := URLSuffix{}

	constructors := map[string]func() interface{}{
		"openai":     func() interface{} { return NewOpenAIModel(base, suffix) },
		"anthropic":  func() interface{} { return NewAnthropicModel(base, suffix) },
		"xai":        func() interface{} { return NewXAIModel(base, suffix) },
		"mistral":    func() interface{} { return NewMistralModel(base, suffix) },
		"perplexity": func() interface{} { return NewPerplexityModel(base, suffix) },
		"togetherai": func() interface{} { return NewTogetherAIModel(base, suffix) },
		"replicate":  func() interface{} { return NewReplicateModel(base, suffix) },
		"upstage":    func() interface{} { return NewUpstageModel(base, suffix) },
		"n1n":        func() interface{} { return NewN1NModel(base, suffix) },
		"novita":     func() interface{} { return NewNovitaModel(base, suffix) },
	}

	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			if model := construct(); model == nil {
				t.Fatalf("%s constructor returned nil", name)
			}
		})
	}
}
