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
  IAssertionReviewQueue,
  IAssertionReviewResult,
  IOntologyModel,
  IOntologyModelImpact,
  IOntologyModelPublishCheck,
  IOntologyModelTrial,
  IOntologyModelVersion,
  IOntologyModelVersionDiff,
} from '@/interfaces/database/ontology-model';
import {
  IAssertionReviewRequestBody,
  IAssertionReviewRequestParams,
} from '@/interfaces/request/ontology-model';
import i18n from '@/locales/config';
import {
  checkOntologyModel,
  diffOntologyModelVersions,
  listAssertionReview,
  listOntologyModelDatasets,
  listOntologyModelVersions,
  listOntologyModels,
  publishOntologyModel,
  rejectOntologyModel,
  retireOntologyModel,
  reviewAssertions,
  submitOntologyModelForReview,
  trialOntologyModel,
} from '@/services/ontology-model-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';

export const enum OntologyModelApiAction {
  FetchModels = 'fetchOntologyModels',
  FetchVersions = 'fetchOntologyModelVersions',
  FetchVersionDiff = 'fetchOntologyModelVersionDiff',
  CheckModel = 'checkOntologyModel',
  SubmitForReview = 'submitOntologyModelForReview',
  Publish = 'publishOntologyModel',
  Reject = 'rejectOntologyModel',
  Retire = 'retireOntologyModel',
  FetchImpact = 'fetchOntologyModelImpact',
  Trial = 'trialOntologyModel',
  ReviewAssertions = 'reviewAssertions',
}

export const OntologyModelKeys = {
  all: () => [OntologyModelApiAction.FetchModels] as const,
  versions: (id?: string) =>
    [OntologyModelApiAction.FetchVersions, id] as const,
  diff: (id?: string, from?: number, to?: number) =>
    [OntologyModelApiAction.FetchVersionDiff, { id, from, to }] as const,
  datasets: (id?: string) => [OntologyModelApiAction.FetchImpact, id] as const,
  assertionReview: (id?: string, params?: IAssertionReviewRequestParams) =>
    [OntologyModelApiAction.ReviewAssertions, id, params] as const,
  /**
   * The queue regardless of its filters. A verdict changes which rows the
   * reader is looking at — the accepted one leaves an "unreviewed" queue — so
   * every filtered view of the same model has to be refetched together.
   */
  allAssertionReviews: (id?: string) =>
    [OntologyModelApiAction.ReviewAssertions, id] as const,
};

/**
 * A lifecycle response, normalised.
 *
 * A refusal is not a transport failure: the server answers with the same check
 * payload it would have returned on success, so the dialog that explains why a
 * publish was blocked and the dialog that confirms one are the same dialog.
 */
export interface IOntologyModelLifecycleResult {
  ok: boolean;
  check?: IOntologyModelPublishCheck;
  message?: string;
}

/**
 * A check payload crosses a version boundary: a server that marshals an empty
 * finding list as `null` is describing "nothing stands in the way", and the
 * dialog reads those lists by length. Coercing here keeps that shape out of the
 * components, which then only ever see arrays.
 */
const normalizePublishCheck = (
  check: IOntologyModelPublishCheck | undefined,
): IOntologyModelPublishCheck | undefined =>
  check
    ? {
        ...check,
        blockers: check.blockers ?? [],
        warnings: check.warnings ?? [],
      }
    : undefined;

const normalizeLifecycleResult = (
  result: { code?: number; message?: string; data?: unknown } | undefined,
): IOntologyModelLifecycleResult => {
  if (result?.code === 0) {
    return {
      ok: true,
      check: normalizePublishCheck(
        result.data as IOntologyModelPublishCheck | undefined,
      ),
    };
  }
  return {
    ok: false,
    check: normalizePublishCheck(
      result?.data as IOntologyModelPublishCheck | undefined,
    ),
    message: result?.message,
  };
};

/**
 * A review outcome, normalised.
 *
 * Unlike a lifecycle call, a failure here is usually partial: the transport
 * succeeded and some rows were written. The component needs the per-row reasons
 * to keep exactly those rows selected, so the payload survives normalisation
 * instead of being collapsed into a message.
 */
export interface IAssertionReviewOutcome {
  ok: boolean;
  result?: IAssertionReviewResult;
  message?: string;
}

const normalizeAssertionReviewResult = (
  result: { code?: number; message?: string; data?: unknown } | undefined,
): IAssertionReviewOutcome => {
  const payload = result?.data as IAssertionReviewResult | undefined;
  const normalized = payload
    ? { ...payload, failed: payload.failed ?? [] }
    : undefined;
  if (result?.code === 0) {
    return { ok: true, result: normalized };
  }
  return { ok: false, result: normalized, message: result?.message };
};

export const useFetchOntologyModels = () => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IOntologyModel[]>({
    queryKey: OntologyModelKeys.all(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listOntologyModels();
      return (data?.data?.models ?? []) as IOntologyModel[];
    },
  });

  return { models: data ?? [], loading, refetch };
};

export const useFetchOntologyModelVersions = (id?: string) => {
  const { data, isFetching: loading } = useQuery<IOntologyModelVersion[]>({
    queryKey: OntologyModelKeys.versions(id),
    initialData: [],
    enabled: !!id,
    gcTime: 0,
    queryFn: async () => {
      if (!id) return [];
      const { data } = await listOntologyModelVersions(id);
      return (data?.data?.versions ?? []) as IOntologyModelVersion[];
    },
  });

  return { versions: data ?? [], loading };
};

/**
 * Loads the datasets a model affects, on demand.
 *
 * It is fetched only when the publisher opens the drill-down rather than with
 * the listing: the listing shows every model at once, and the datasets are a
 * decision about one of them.
 */
export const useFetchOntologyModelImpact = (id?: string) => {
  const { data, isFetching: loading } = useQuery<IOntologyModelImpact>({
    queryKey: OntologyModelKeys.datasets(id),
    enabled: !!id,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listOntologyModelDatasets(id!);
      return data?.data as IOntologyModelImpact;
    },
  });

  return { impact: data, loading };
};

/**
 * Reads the assertions of a model that still need a verdict.
 *
 * It is fetch-on-open like the inspection beside it — a queue nobody is reading
 * is a request nobody needs — but it keeps no cache: a verdict moves rows out of
 * the queue, so a stale page would show rows that are already settled.
 */
export const useFetchAssertionReview = (
  id?: string,
  params?: IAssertionReviewRequestParams,
  enabled = true,
) => {
  const {
    data,
    isFetching: loading,
    error,
  } = useQuery<IAssertionReviewQueue>({
    queryKey: OntologyModelKeys.assertionReview(id, params),
    enabled: !!id && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listAssertionReview(id!, params ?? {});
      return data?.data as IAssertionReviewQueue;
    },
  });

  return { queue: data, loading, error };
};

export const useFetchOntologyModelVersionDiff = (
  id?: string,
  from = 0,
  to = 0,
) => {
  const { data, isFetching: loading } = useQuery<IOntologyModelVersionDiff>({
    queryKey: OntologyModelKeys.diff(id, from, to),
    enabled: !!id && (from > 0 || to > 0),
    gcTime: 0,
    queryFn: async () => {
      const { data } = await diffOntologyModelVersions(id!, from, to);
      return data?.data as IOntologyModelVersionDiff;
    },
  });

  return { diff: data, loading };
};

/** Re-reads the pre-publish findings without publishing anything. */
export const useCheckOntologyModel = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.CheckModel],
    mutationFn: async (id: string) => {
      const { data } = await checkOntologyModel(id);
      return normalizeLifecycleResult(data);
    },
  });

  return { loading, checkModel: mutateAsync };
};

const useLifecycleInvalidate = () => {
  const queryClient = useQueryClient();
  return useCallback(() => {
    queryClient.invalidateQueries({
      queryKey: OntologyModelKeys.all(),
    });
  }, [queryClient]);
};

export const useSubmitOntologyModelForReview = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.SubmitForReview],
    mutationFn: async (id: string) => {
      const { data } = await submitOntologyModelForReview(id);
      const result = normalizeLifecycleResult(data);
      if (result.ok) {
        invalidate();
      }
      return result;
    },
  });

  return { loading, submitForReview: mutateAsync };
};

/**
 * Rehearses a model change over real documents.
 *
 * Nothing is invalidated on success: a rehearsal reads and compiles, it does
 * not move the lifecycle, so the library on screen is still current.
 */
export const useTrialOntologyModel = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.Trial],
    mutationFn: async (id: string) => {
      const { data } = await trialOntologyModel(id);
      if (data?.code === 0) {
        return { ok: true, trial: data.data as IOntologyModelTrial };
      }
      return { ok: false, message: data?.message };
    },
  });

  return { loading, runTrial: mutateAsync };
};

export const usePublishOntologyModel = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.Publish],
    mutationFn: async ({
      id,
      note,
      force,
    }: {
      id: string;
      note?: string;
      force?: boolean;
    }) => {
      const { data } = await publishOntologyModel(id, { note, force });
      const result = normalizeLifecycleResult(data);
      if (result.ok) {
        invalidate();
      }
      return result;
    },
  });

  return { loading, publishModel: mutateAsync };
};

/**
 * Records verdicts on a batch of assertions.
 *
 * It does not use the lifecycle invalidation: judging rows changes no version
 * and no dataset, only which rows the queue still holds, so refetching the model
 * library on every accept would be noise.
 */
export const useReviewAssertions = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.ReviewAssertions],
    mutationFn: async ({
      id,
      items,
    }: {
      id: string;
      items: IAssertionReviewRequestBody['items'];
    }) => {
      const { data } = await reviewAssertions(id, { items });
      const outcome = normalizeAssertionReviewResult(data);
      // A partially failed batch still moved the rows that were written, so the
      // queue is refetched for those even when the rest have to be retried.
      if ((outcome.result?.updated ?? 0) > 0) {
        queryClient.invalidateQueries({
          queryKey: OntologyModelKeys.allAssertionReviews(id),
        });
      }
      return outcome;
    },
  });

  return { loading, reviewAssertions: mutateAsync };
};

export const useRejectOntologyModel = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.Reject],
    mutationFn: async ({ id, reason }: { id: string; reason: string }) => {
      const { data } = await rejectOntologyModel(id, { reason });
      const result = normalizeLifecycleResult(data);
      if (result.ok) {
        invalidate();
      }
      return result;
    },
  });

  return { loading, rejectModel: mutateAsync };
};

export const useRetireOntologyModel = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.Retire],
    mutationFn: async (id: string) => {
      const { data } = await retireOntologyModel(id);
      const result = normalizeLifecycleResult(data);
      if (result.ok) {
        invalidate();
      }
      return result;
    },
  });

  return { loading, retireModel: mutateAsync };
};

export const ontologyModelStatusLabel = (status: string) => {
  switch (status) {
    case 'published':
      return i18n.t('knowledgeCompilation.ontologyModel.status.published');
    case 'review':
      return i18n.t('knowledgeCompilation.ontologyModel.status.review');
    case 'retired':
      return i18n.t('knowledgeCompilation.ontologyModel.status.retired');
    default:
      return i18n.t('knowledgeCompilation.ontologyModel.status.draft');
  }
};
