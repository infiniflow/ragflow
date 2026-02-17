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

# Create a dataset
echo -e "\n-- Create a dataset"
curl --request POST \
     --url http://localhost:9380/api/v1/datasets \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
      "name": "test"
      }'

# Update the dataset
echo -e "\n-- Update the dataset"
curl --request PUT \
     --url http://localhost:9380/api/v1/datasets/2e898768a0bc11efb46a0242ac120006 \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '
     {
          "name": "updated_dataset"
     }'

# List datasets
echo -e "\n-- List datasets"
curl --request GET \
     --url http://127.0.0.1:9380/api/v1/datasets \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm'

# Delete datasets
echo -e "\n-- Delete datasets"
curl --request DELETE \
     --url http://localhost:9380/api/v1/datasets \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
     "ids": ["301298b8a0bc11efa0440242ac120006"]
     }'
