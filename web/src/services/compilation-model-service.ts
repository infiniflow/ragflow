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

import { ICompilationModelInspectParams } from '@/interfaces/database/compilation-model';
import api from '@/utils/api';
import request from '@/utils/next-request';

export const listCompilationModels = (params?: { kind?: string }) =>
  request.get(api.compilationModels, { params });

export const checkCompilationModel = (id: string) =>
  request.get(api.compilationModelCheck(id));

export const submitCompilationModelForReview = (id: string) =>
  request.post(api.compilationModelReview(id));

export const publishCompilationModel = (
  id: string,
  data: { note?: string; force?: boolean },
) => request.post(api.compilationModelPublish(id), data);

export const rejectCompilationModel = (id: string, data: { reason: string }) =>
  request.post(api.compilationModelReject(id), data);

export const retireCompilationModel = (id: string) =>
  request.post(api.compilationModelRetire(id));

export const listCompilationModelVersions = (id: string) =>
  request.get(api.compilationModelVersions(id));

export const diffCompilationModelVersions = (
  id: string,
  from: number,
  to: number,
) => request.get(api.compilationModelVersionDiff(id), { params: { from, to } });

export const listCompilationModelDatasets = (id: string) =>
  request.get(api.compilationModelDatasets(id));

/**
 * Rehearses a model change over real documents.
 *
 * It is a POST because it is not a read: the server compiles sampled documents
 * with the draft model, which costs an LLM pass per document.
 */
export const trialCompilationModel = (id: string) =>
  request.post(api.compilationModelTrial(id), {});

/**
 * Reads what the model already extracted, over the datasets that compile with
 * it. A read, unlike the rehearsal above: it calls no model and costs one engine
 * query per dataset.
 */
export const inspectCompilationModelInstances = (
  id: string,
  params: ICompilationModelInspectParams,
) => request.get(api.compilationModelInstances(id), { params });
