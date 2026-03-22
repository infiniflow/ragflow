#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

# You can replace the dataset_id, document_id and chunk_id with your own ids.
DATASET_ID="4604bf0224ff11f1909d7513c15e4608"
DOCUMENT_ID="38bf5572250011f1909d7513c15e4608"
API_KEY="ragflow-Ih3xBYaSqioDcPUl4rNpYBNjv5OQ3I9zDrnptCgxK1c"
HOST="http://127.0.0.1:9380"


## List chunks to see the tag_kwd field
echo -e "\n-- List chunks"
curl --request GET \
     --url "$HOST/api/v1/datasets/$DATASET_ID/documents/$DOCUMENT_ID/chunks" \
     --header "Authorization: Bearer $API_KEY"
