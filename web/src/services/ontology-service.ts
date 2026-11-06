/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { IOntologyRetrievalRequestBody } from '@/interfaces/request/ontology-retrieval';
import api from '@/utils/api';
import request from '@/utils/next-request';

/**
 * Runs one knowledge-layer lookup.
 *
 * A POST because it is not a read of a stored resource: the server searches the
 * compiled rows for this question, which is the work a turn does. Nothing is
 * written, so a preview may be run as often as a reader wants.
 */
export const retrieveOntologyAssertions = (
  data: IOntologyRetrievalRequestBody,
) => request.post(api.ontologyRetrieval, data);
