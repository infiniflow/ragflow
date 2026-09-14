//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// Warranties, INCLUDING THE WARRANTIES OF MERCHANTABILITY AND
// FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//

package parser

// DefaultMarkdownDelimiter is the flow parser's default markdown delimiter
// set, used when generating/loading the golden baseline and when the markdown
// alignment test normalizes delimiters. It is a non-test symbol so both the
// production parsers and the alignment tests share one source of truth.
const DefaultMarkdownDelimiter = "\n!?;。；！？"

// DefaultTextCodeDelimiter is the flow parser's default text&code delimiter
// set, used by TextParser and when the text&code alignment test normalizes
// delimiters. It is a non-test symbol so both the production parser and the
// alignment tests share one source of truth (no duplicate hard-coded copy in
// text_parser.go, which would otherwise drift silently).
const DefaultTextCodeDelimiter = "\n!?;。；！？"
