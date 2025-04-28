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

#    常量文件
#    • API_VERSION就是上面介绍的api的版本
#    • SERVICE_CONF是加载配置文件
#    • REQUEST_WAIT_SEC和REQUEST_MAX_WAIT_SEC两个参数没啥用
#    • DATASET_NAME_LIMIT 限制数据集的名称大小
    
NAME_LENGTH_LIMIT = 2 ** 10

IMG_BASE64_PREFIX = 'data:image/png;base64,'

SERVICE_CONF = "service_conf.yaml"

API_VERSION = "v1"
RAG_FLOW_SERVICE_NAME = "ragflow"
REQUEST_WAIT_SEC = 2
REQUEST_MAX_WAIT_SEC = 300

DATASET_NAME_LIMIT = 128
