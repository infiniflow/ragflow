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

package dao

import (
	"database/sql/driver"
	"regexp"
	"sync"

	sqlite3 "github.com/glebarez/go-sqlite"
)

var (
	sqliteRegexpOnce sync.Once
	sqliteRegexpErr  error
)

// registerSQLiteRegexp teaches SQLite the REGEXP operator that the DAO's
// hand-written SQL uses (user_canvas tag filtering). MySQL and Vastbase have
// it natively, so only the desktop build's store needs the callback.
//
// SQLite calls the function as `regexp(pattern, value)` for the infix form
// `value REGEXP pattern` - the arguments are reversed compared with how the
// expression is written.
func registerSQLiteRegexp() error {
	sqliteRegexpOnce.Do(func() {
		var mu sync.Mutex
		compiled := make(map[string]*regexp.Regexp)

		sqliteRegexpErr = sqlite3.RegisterScalarFunction("regexp", 2,
			func(_ *sqlite3.FunctionContext, args []driver.Value) (driver.Value, error) {
				if len(args) != 2 {
					return int64(0), nil
				}
				pattern, patternOK := args[0].(string)
				value, valueOK := args[1].(string)
				if !patternOK || !valueOK {
					return int64(0), nil
				}

				mu.Lock()
				re, ok := compiled[pattern]
				if !ok {
					var err error
					re, err = regexp.Compile(pattern)
					if err != nil {
						mu.Unlock()
						return int64(0), nil
					}
					compiled[pattern] = re
				}
				mu.Unlock()

				if re.MatchString(value) {
					return int64(1), nil
				}
				return int64(0), nil
			})
	})
	return sqliteRegexpErr
}
