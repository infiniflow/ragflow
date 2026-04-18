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

package cli

import "fmt"

type ResponseIf interface {
	Type() string
	PrintOut()
	TimeCost() float64
	SetOutputFormat(format OutputFormat)
}

type CommonResponse struct {
	Code         int                      `json:"code"`
	Data         []map[string]interface{} `json:"data"`
	Message      string                   `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *CommonResponse) Type() string {
	return "common"
}

func (r *CommonResponse) TimeCost() float64 {
	return r.Duration
}

func (r *CommonResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *CommonResponse) PrintOut() {
	if r.Code == 0 {
		PrintTableSimpleByFormat(r.Data, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type CommonDataResponse struct {
	Code         int                    `json:"code"`
	Data         map[string]interface{} `json:"data"`
	Message      string                 `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *CommonDataResponse) Type() string {
	return "show"
}

func (r *CommonDataResponse) TimeCost() float64 {
	return r.Duration
}

func (r *CommonDataResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *CommonDataResponse) PrintOut() {
	if r.Code == 0 {
		table := make([]map[string]interface{}, 0)
		table = append(table, r.Data)
		PrintTableSimpleByFormat(table, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type SimpleResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *SimpleResponse) Type() string {
	return "simple"
}

func (r *SimpleResponse) TimeCost() float64 {
	return r.Duration
}

func (r *SimpleResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *SimpleResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println("SUCCESS")
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type MessageResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *MessageResponse) Type() string {
	return "message"
}

func (r *MessageResponse) TimeCost() float64 {
	return r.Duration
}

func (r *MessageResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *MessageResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println(r.Message)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type StreamMessageResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *StreamMessageResponse) Type() string {
	return "stream_message"
}

func (r *StreamMessageResponse) TimeCost() float64 {
	return r.Duration
}

func (r *StreamMessageResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *StreamMessageResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type RegisterResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *RegisterResponse) Type() string {
	return "register"
}

func (r *RegisterResponse) TimeCost() float64 {
	return r.Duration
}

func (r *RegisterResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *RegisterResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println("Register successfully")
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

type BenchmarkResponse struct {
	Code         int     `json:"code"`
	Duration     float64 `json:"duration"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	Concurrency  int
	OutputFormat OutputFormat
}

func (r *BenchmarkResponse) Type() string {
	return "benchmark"
}

func (r *BenchmarkResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *BenchmarkResponse) PrintOut() {
	if r.Code != 0 {
		fmt.Printf("ERROR, Code: %d\n", r.Code)
		return
	}

	iterations := r.SuccessCount + r.FailureCount
	if r.Concurrency == 1 {
		if iterations == 1 {
			fmt.Printf("Latency: %fs\n", r.Duration)
		} else {
			fmt.Printf("Latency: %fs, QPS: %.1f, SUCCESS: %d, FAILURE: %d\n", r.Duration, float64(iterations)/r.Duration, r.SuccessCount, r.FailureCount)
		}
	} else {
		fmt.Printf("Concurrency: %d, Latency: %fs, QPS: %.1f, SUCCESS: %d, FAILURE: %d\n", r.Concurrency, r.Duration, float64(iterations)/r.Duration, r.SuccessCount, r.FailureCount)
	}
}

func (r *BenchmarkResponse) TimeCost() float64 {
	return r.Duration
}

type KeyValueResponse struct {
	Code         int    `json:"code"`
	Key          string `json:"key"`
	Value        string `json:"data"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *KeyValueResponse) Type() string {
	return "data"
}

func (r *KeyValueResponse) TimeCost() float64 {
	return r.Duration
}

func (r *KeyValueResponse) SetOutputFormat(format OutputFormat) {
	r.OutputFormat = format
}

func (r *KeyValueResponse) PrintOut() {
	if r.Code == 0 {
		table := make([]map[string]interface{}, 0)
		// insert r.key and r.value into table
		table = append(table, map[string]interface{}{
			"key":   r.Key,
			"value": r.Value,
		})
		PrintTableSimpleByFormat(table, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d\n", r.Code)
	}
}

// ==================== ContextEngine Commands ====================

// ContextListResponse represents the response for ls command
type ContextListResponse struct {
	Code         int                      `json:"code"`
	Data         []map[string]interface{} `json:"data"`
	Message      string                   `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ContextListResponse) Type() string                        { return "ce_ls" }
func (r *ContextListResponse) TimeCost() float64                   { return r.Duration }
func (r *ContextListResponse) SetOutputFormat(format OutputFormat) { r.OutputFormat = format }
func (r *ContextListResponse) PrintOut() {
	if r.Code == 0 {
		PrintTableSimpleByFormat(r.Data, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

// ContextSearchResponse represents the response for search command
type ContextSearchResponse struct {
	Code         int                      `json:"code"`
	Data         []map[string]interface{} `json:"data"`
	Total        int                      `json:"total"`
	Message      string                   `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ContextSearchResponse) Type() string                        { return "ce_search" }
func (r *ContextSearchResponse) TimeCost() float64                   { return r.Duration }
func (r *ContextSearchResponse) SetOutputFormat(format OutputFormat) { r.OutputFormat = format }
func (r *ContextSearchResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Printf("Found %d results:\n", r.Total)
		PrintTableSimpleByFormat(r.Data, r.OutputFormat)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

// ContextCatResponse represents the response for cat command
type ContextCatResponse struct {
	Code         int          `json:"code"`
	Content      string       `json:"content"`
	Message      string       `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ContextCatResponse) Type() string                        { return "ce_cat" }
func (r *ContextCatResponse) TimeCost() float64                   { return r.Duration }
func (r *ContextCatResponse) SetOutputFormat(format OutputFormat) { r.OutputFormat = format }
func (r *ContextCatResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println(r.Content)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

// ContextMountResponse represents the response for mount command
type ContextMountResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ContextMountResponse) Type() string                        { return "ce_mount" }
func (r *ContextMountResponse) TimeCost() float64                   { return r.Duration }
func (r *ContextMountResponse) SetOutputFormat(format OutputFormat) { r.OutputFormat = format }
func (r *ContextMountResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println(r.Message)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}

// ContextUnmountResponse represents the response for unmount command
type ContextUnmountResponse struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Duration     float64
	OutputFormat OutputFormat
}

func (r *ContextUnmountResponse) Type() string                        { return "ce_unmount" }
func (r *ContextUnmountResponse) TimeCost() float64                   { return r.Duration }
func (r *ContextUnmountResponse) SetOutputFormat(format OutputFormat) { r.OutputFormat = format }
func (r *ContextUnmountResponse) PrintOut() {
	if r.Code == 0 {
		fmt.Println(r.Message)
	} else {
		fmt.Println("ERROR")
		fmt.Printf("%d, %s\n", r.Code, r.Message)
	}
}
