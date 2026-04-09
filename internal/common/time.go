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

package common

import (
	"time"
)

// DeltaSeconds calculates seconds elapsed from a given date string to now.
//
// Args:
//   dateString: Date string in "YYYY-MM-DD HH:MM:SS" format
//
// Returns:
//   float64: Number of seconds between the given date and current time
//
// Example:
//   DeltaSeconds("2024-01-01 12:00:00")
//   // Returns: 3600.0 (if current time is 2024-01-01 13:00:00)
func DeltaSeconds(dateString string) (float64, error) {
	const layout = "2006-01-02 15:04:05"
	dt, err := time.ParseInLocation(layout, dateString, time.Local)
	if err != nil {
		return 0, err
	}
	return time.Since(dt).Seconds(), nil
}
