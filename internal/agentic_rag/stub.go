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

//go:build !ee

package agentic_rag

// The community edition ships no locate/served-unread watchdogs: they live in
// the *_ee.go files. These stubs keep the shared agent loop compiling, and both
// observers return "" so no reminder is ever appended to a tool result.

// locateWatch and servedUnreadWatch are the EE run watchdogs. In OSS they are
// inert: they hold no state and never nudge the model.
type locateWatch struct{}
type servedUnreadWatch struct{}

func newLocateWatch() *locateWatch { return &locateWatch{} }

func newServedUnreadWatch(served *servedLedger, deep *docIDLedger) *servedUnreadWatch {
	return &servedUnreadWatch{}
}

// observe always returns "": the OSS build never appends a watchdog reminder.
func (*locateWatch) observe(name string) string       { return "" }
func (*servedUnreadWatch) observe(name string) string { return "" }

// markSemanticAvailable is a no-op: the OSS watchdog has no state to record.
func (*locateWatch) markSemanticAvailable() {}
