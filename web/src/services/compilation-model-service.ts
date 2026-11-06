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
import {
  IAssertionReviewRequestBody,
  IAssertionReviewRequestParams,
} from '@/interfaces/request/compilation-model';
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

/**
 * Reads the assertions of a model that still need a verdict.
 *
 * A read like the inspection above, and it shares its shape on purpose: the
 * queue is the same rows filtered by review state, so a row seen in one panel is
 * recognisable in the other.
 */
export const listAssertionReview = (
  id: string,
  params: IAssertionReviewRequestParams,
) => request.get(api.assertionReview(id), { params });

/**
 * Records verdicts on the assertions of a model.
 *
 * One request carries the whole selection and the server answers per row, which
 * is why a partial result comes back with the rows it could not write rather
 * than as a transport failure.
 */
export const reviewAssertions = (
  id: string,
  data: IAssertionReviewRequestBody,
) => request.post(api.assertionReview(id), data);
