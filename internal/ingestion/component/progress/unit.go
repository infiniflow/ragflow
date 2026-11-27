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

package progress

// Structured progress unit names. They are the keys consumed by the ingestion
// log and the progress snapshot, and must stay in sync with the unit labels in
// the web locales (knowledgeDetails.ingestionProgress.units).
const (
	UnitPageParse  = "page:parse"
	UnitImageCrop  = "image:crop"
	UnitChunkFinal = "chunk:final"
	UnitChunkEmbed = "chunk:embed"
)
