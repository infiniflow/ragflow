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

package channels

import (
	"errors"
	"net/url"
)

// redactURLError strips the query string from transport errors. WeCom carries
// the access token / corp secret in the request URL query, and *url.Error keeps
// the full URL in its message, so raw errors must not reach the log sinks.
func redactURLError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	parsed, parseErr := url.Parse(urlErr.URL)
	if parseErr != nil {
		return urlErr
	}
	parsed.RawQuery = ""
	return &url.Error{Op: urlErr.Op, URL: parsed.String(), Err: redactURLError(urlErr.Err)}
}
