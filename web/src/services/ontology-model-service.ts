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

import {
  IAssertionReviewRequestBody,
  IAssertionReviewRequestParams,
} from '@/interfaces/request/ontology-model';
import api from '@/utils/api';
import request from '@/utils/next-request';

export const listOntologyModels = () => request.get(api.ontologyModels);

export const checkOntologyModel = (id: string) =>
  request.get(api.ontologyModelCheck(id));

export const submitOntologyModelForReview = (id: string) =>
  request.post(api.ontologyModelReview(id));

export const publishOntologyModel = (
  id: string,
  data: { note?: string; force?: boolean },
) => request.post(api.ontologyModelPublish(id), data);

export const rejectOntologyModel = (id: string, data: { reason: string }) =>
  request.post(api.ontologyModelReject(id), data);

export const retireOntologyModel = (id: string) =>
  request.post(api.ontologyModelRetire(id));

export const listOntologyModelVersions = (id: string) =>
  request.get(api.ontologyModelVersions(id));

export const diffOntologyModelVersions = (
  id: string,
  from: number,
  to: number,
) => request.get(api.ontologyModelVersionDiff(id), { params: { from, to } });

export const listOntologyModelDatasets = (id: string) =>
  request.get(api.ontologyModelDatasets(id));

/**
 * Rehearses a model change over real documents.
 *
 * It is a POST because it is not a read: the server compiles sampled documents
 * with the draft model, which costs an LLM pass per document.
 */
export const trialOntologyModel = (id: string) =>
  request.post(api.ontologyModelTrial(id), {});

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

/**
 * The model's compiled actions with contract, binding and verdict.
 *
 * One request for the three, because the workbench shows them in one row and
 * fetching them apart would let the row's cells disagree on screen.
 */
export const listModelActions = (id: string) =>
  request.get(api.ontologyModelActions(id));

/**
 * Edits one action's contract.
 *
 * `reason` is required by the server: a contract edit changes what the system may
 * do, so the audit trail records why, not only what.
 */
export const updateActionContract = (
  id: string,
  actionId: string,
  data: {
    contract?: Record<string, unknown>;
    description?: string;
    reason: string;
  },
  params?: { dataset_id?: string },
) => request.patch(api.ontologyModelAction(id, actionId), data, { params });

export const listActionBindings = (id: string) =>
  request.get(api.ontologyModelActionBindings(id));

export const saveActionBinding = (
  id: string,
  actionName: string,
  data: {
    mcp_server: string;
    mcp_tool: string;
    arg_mapping?: Record<string, string>;
    read_only?: boolean;
    enabled?: boolean;
    qa_callable?: boolean;
    /** Required when the binding becomes agent-callable or is declared writable. */
    risk_ack?: boolean;
    note?: string;
    dataset_id?: string;
  },
) => request.put(api.ontologyModelActionBinding(id, actionName), data);

export const deleteActionBinding = (id: string, actionName: string) =>
  request.delete(api.ontologyModelActionBinding(id, actionName));

/** Reports the verdict and the exact arguments without calling anything. */
export const dryRunAction = (
  id: string,
  actionName: string,
  data: { arguments?: Record<string, unknown> } = {},
) => request.post(api.ontologyModelActionDryRun(id, actionName), data);

/**
 * Runs one action.
 *
 * The server decides: this call may come back with `executed: false` and the
 * reasons, which is a normal answer rather than an error to retry.
 */
export const executeAction = (
  id: string,
  actionName: string,
  data: { arguments?: Record<string, unknown>; reason: string },
) => request.post(api.ontologyModelActionExecute(id, actionName), data);

export const listActionExecutions = (
  id: string,
  params?: {
    page?: number;
    page_size?: number;
    action_name?: string;
    verdict?: string;
    status?: string;
    mode?: string;
    from?: number;
    to?: number;
  },
) => request.get(api.ontologyModelActionExecutions(id), { params });

export const listActionContractEdits = (
  id: string,
  actionName: string,
  params?: { page?: number; page_size?: number },
) =>
  request.get(api.ontologyModelActionContractEdits(id, actionName), {
    params,
  });

/**
 * Stamps the contract columns on action rows compiled before they existed.
 *
 * Idempotent by design, which is why it is a button and not a one-off script:
 * running it twice does nothing, and running it once is not a migration someone
 * has to remember to apply to a database by hand.
 */
export const backfillActionContracts = (id: string) =>
  request.post(api.ontologyModelActionBackfill(id), {});
