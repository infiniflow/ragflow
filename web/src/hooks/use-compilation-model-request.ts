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
  ICompilationModel,
  ICompilationModelImpact,
  ICompilationModelInspect,
  ICompilationModelInspectParams,
  ICompilationModelPublishCheck,
  ICompilationModelTrial,
  ICompilationModelVersion,
  ICompilationModelVersionDiff,
} from '@/interfaces/database/compilation-model';
import {
  IAssertionReviewRequestBody,
  IAssertionReviewRequestParams,
} from '@/interfaces/request/compilation-model';
import i18n from '@/locales/config';
import {
  checkCompilationModel,
  diffCompilationModelVersions,
  inspectCompilationModelInstances,
  listAssertionReview,
  listCompilationModelDatasets,
  listCompilationModelVersions,
  listCompilationModels,
  publishCompilationModel,
  rejectCompilationModel,
  retireCompilationModel,
  reviewAssertions,
  submitCompilationModelForReview,
  trialCompilationModel,
} from '@/services/compilation-model-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';

export const enum CompilationModelApiAction {
  FetchModels = 'fetchCompilationModels',
  FetchVersions = 'fetchCompilationModelVersions',
  FetchVersionDiff = 'fetchCompilationModelVersionDiff',
  CheckModel = 'checkCompilationModel',
  SubmitForReview = 'submitCompilationModelForReview',
  Publish = 'publishCompilationModel',
  Reject = 'rejectCompilationModel',
  Retire = 'retireCompilationModel',
  FetchImpact = 'fetchCompilationModelImpact',
  Trial = 'trialCompilationModel',
  Inspect = 'inspectCompilationModelInstances',
  ReviewAssertions = 'reviewAssertions',
}

export const CompilationModelKeys = {
  list: (kind?: string) =>
    [CompilationModelApiAction.FetchModels, { kind }] as const,
  all: () => [CompilationModelApiAction.FetchModels] as const,
  versions: (id?: string) =>
    [CompilationModelApiAction.FetchVersions, id] as const,
  diff: (id?: string, from?: number, to?: number) =>
    [CompilationModelApiAction.FetchVersionDiff, { id, from, to }] as const,
  datasets: (id?: string) =>
    [CompilationModelApiAction.FetchImpact, id] as const,
  inspect: (id?: string, params?: ICompilationModelInspectParams) =>
    [CompilationModelApiAction.Inspect, id, params] as const,
  assertionReview: (id?: string, params?: IAssertionReviewRequestParams) =>
    [CompilationModelApiAction.ReviewAssertions, id, params] as const,
  /**
   * The queue regardless of its filters. A verdict changes which rows the
   * reader is looking at — the accepted one leaves an "unreviewed" queue — so
   * every filtered view of the same model has to be refetched together.
   */
  allAssertionReviews: (id?: string) =>
    [CompilationModelApiAction.ReviewAssertions, id] as const,
};

/**
 * A lifecycle response, normalised.
 *
 * A refusal is not a transport failure: the server answers with the same check
 * payload it would have returned on success, so the dialog that explains why a
 * publish was blocked and the dialog that confirms one are the same dialog.
 */
export interface ICompilationModelLifecycleResult {
  ok: boolean;
  check?: ICompilationModelPublishCheck;
  message?: string;
}

/**
 * A check payload crosses a version boundary: a server that marshals an empty
 * finding list as `null` is describing "nothing stands in the way", and the
 * dialog reads those lists by length. Coercing here keeps that shape out of the
 * components, which then only ever see arrays.
 */
const normalizePublishCheck = (
  check: ICompilationModelPublishCheck | undefined,
): ICompilationModelPublishCheck | undefined =>
  check
    ? {
        ...check,
        blockers: check.blockers ?? [],
        warnings: check.warnings ?? [],
      }
    : undefined;

const normalizeLifecycleResult = (
  result: { code?: number; message?: string; data?: unknown } | undefined,
): ICompilationModelLifecycleResult => {
  if (result?.code === 0) {
    return {
      ok: true,
      check: normalizePublishCheck(
        result.data as ICompilationModelPublishCheck | undefined,
      ),
    };
  }
  return {
    ok: false,
    check: normalizePublishCheck(
      result?.data as ICompilationModelPublishCheck | undefined,
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

export const useFetchCompilationModels = (kind?: string) => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<ICompilationModel[]>({
    queryKey: CompilationModelKeys.list(kind),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listCompilationModels(kind ? { kind } : undefined);
      return (data?.data?.models ?? []) as ICompilationModel[];
    },
  });

  return { models: data ?? [], loading, refetch };
};

export const useFetchCompilationModelVersions = (id?: string) => {
  const { data, isFetching: loading } = useQuery<ICompilationModelVersion[]>({
    queryKey: CompilationModelKeys.versions(id),
    initialData: [],
    enabled: !!id,
    gcTime: 0,
    queryFn: async () => {
      if (!id) return [];
      const { data } = await listCompilationModelVersions(id);
      return (data?.data?.versions ?? []) as ICompilationModelVersion[];
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
export const useFetchCompilationModelImpact = (id?: string) => {
  const { data, isFetching: loading } = useQuery<ICompilationModelImpact>({
    queryKey: CompilationModelKeys.datasets(id),
    enabled: !!id,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listCompilationModelDatasets(id!);
      return data?.data as ICompilationModelImpact;
    },
  });

  return { impact: data, loading };
};

/**
 * Reads what the model extracted: one class's instances, or one property's
 * assertions, over the datasets that compile with it.
 *
 * An unfiltered read is the opening state, not an empty one: with nothing
 * selected the answer is every row of the requested kind, which is what "what
 * did this model produce at all" means. So the only thing that can hold the
 * query back is the missing model.
 *
 * It is a corpus question, not a dataset one — a model is one template over many
 * datasets — so the query fans out server-side and every row comes back labelled
 * with the dataset it came from.
 */
export const useFetchCompilationModelInstances = (
  id?: string,
  params?: ICompilationModelInspectParams,
) => {
  const {
    data,
    isFetching: loading,
    error,
  } = useQuery<ICompilationModelInspect>({
    queryKey: CompilationModelKeys.inspect(id, params),
    enabled: !!id,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await inspectCompilationModelInstances(
        id!,
        params ?? {},
      );
      return data?.data as ICompilationModelInspect;
    },
  });

  return { inspect: data, loading, error };
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
    queryKey: CompilationModelKeys.assertionReview(id, params),
    enabled: !!id && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listAssertionReview(id!, params ?? {});
      return data?.data as IAssertionReviewQueue;
    },
  });

  return { queue: data, loading, error };
};

export const useFetchCompilationModelVersionDiff = (
  id?: string,
  from = 0,
  to = 0,
) => {
  const { data, isFetching: loading } = useQuery<ICompilationModelVersionDiff>({
    queryKey: CompilationModelKeys.diff(id, from, to),
    enabled: !!id && (from > 0 || to > 0),
    gcTime: 0,
    queryFn: async () => {
      const { data } = await diffCompilationModelVersions(id!, from, to);
      return data?.data as ICompilationModelVersionDiff;
    },
  });

  return { diff: data, loading };
};

/** Re-reads the pre-publish findings without publishing anything. */
export const useCheckCompilationModel = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [CompilationModelApiAction.CheckModel],
    mutationFn: async (id: string) => {
      const { data } = await checkCompilationModel(id);
      return normalizeLifecycleResult(data);
    },
  });

  return { loading, checkModel: mutateAsync };
};

const useLifecycleInvalidate = () => {
  const queryClient = useQueryClient();
  return useCallback(() => {
    queryClient.invalidateQueries({
      queryKey: CompilationModelKeys.all(),
    });
  }, [queryClient]);
};

export const useSubmitCompilationModelForReview = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [CompilationModelApiAction.SubmitForReview],
    mutationFn: async (id: string) => {
      const { data } = await submitCompilationModelForReview(id);
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
export const useTrialCompilationModel = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [CompilationModelApiAction.Trial],
    mutationFn: async (id: string) => {
      const { data } = await trialCompilationModel(id);
      if (data?.code === 0) {
        return { ok: true, trial: data.data as ICompilationModelTrial };
      }
      return { ok: false, message: data?.message };
    },
  });

  return { loading, runTrial: mutateAsync };
};

export const usePublishCompilationModel = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [CompilationModelApiAction.Publish],
    mutationFn: async ({
      id,
      note,
      force,
    }: {
      id: string;
      note?: string;
      force?: boolean;
    }) => {
      const { data } = await publishCompilationModel(id, { note, force });
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
    mutationKey: [CompilationModelApiAction.ReviewAssertions],
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
          queryKey: CompilationModelKeys.allAssertionReviews(id),
        });
      }
      return outcome;
    },
  });

  return { loading, reviewAssertions: mutateAsync };
};

export const useRejectCompilationModel = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [CompilationModelApiAction.Reject],
    mutationFn: async ({ id, reason }: { id: string; reason: string }) => {
      const { data } = await rejectCompilationModel(id, { reason });
      const result = normalizeLifecycleResult(data);
      if (result.ok) {
        invalidate();
      }
      return result;
    },
  });

  return { loading, rejectModel: mutateAsync };
};

export const useRetireCompilationModel = () => {
  const invalidate = useLifecycleInvalidate();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [CompilationModelApiAction.Retire],
    mutationFn: async (id: string) => {
      const { data } = await retireCompilationModel(id);
      const result = normalizeLifecycleResult(data);
      if (result.ok) {
        invalidate();
      }
      return result;
    },
  });

  return { loading, retireModel: mutateAsync };
};

export const compilationModelStatusLabel = (status: string) => {
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
