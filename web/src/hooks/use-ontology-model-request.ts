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
  IOntologyModelPendingChanges,
  IOntologyModelPublishCheck,
  IOntologyModelVersion,
  IOntologyModelVersionDiff,
} from '@/interfaces/database/ontology-model';
import {
  IAssertionReviewRequestBody,
  IAssertionReviewRequestParams,
} from '@/interfaces/request/ontology-model';
import {
  IOntologyContractExport,
  IOntologyInsight,
} from '@/interfaces/database/ontology-insight';
import i18n from '@/locales/config';
import {
  checkOntologyModel,
  diffOntologyModelVersions,
  dismissDriftFinding,
  getOntologyModelChanges,
  getOntologyModelContract,
  getOntologyModelInsight,
  listAssertionReview,
  listDriftDismissals,
  listOntologyModelDatasets,
  listOntologyModelVersions,
  listOntologyModels,
  publishOntologyModel,
  refreshOntologyModelInsight,
  rejectOntologyModel,
  restoreDriftFinding,
  retireOntologyModel,
  reviewAssertions,
  submitOntologyModelForReview,
} from '@/services/ontology-model-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';

export const enum OntologyModelApiAction {
  FetchModels = 'fetchOntologyModels',
  FetchVersions = 'fetchOntologyModelVersions',
  FetchVersionDiff = 'fetchOntologyModelVersionDiff',
  FetchChanges = 'fetchOntologyModelChanges',
  FetchDismissals = 'fetchDriftDismissals',
  DismissFinding = 'dismissDriftFinding',
  RestoreFinding = 'restoreDriftFinding',
  CheckModel = 'checkOntologyModel',
  SubmitForReview = 'submitOntologyModelForReview',
  Publish = 'publishOntologyModel',
  Reject = 'rejectOntologyModel',
  Retire = 'retireOntologyModel',
  FetchImpact = 'fetchOntologyModelImpact',
  ReviewAssertions = 'reviewAssertions',
  FetchInsight = 'fetchOntologyModelInsight',
  RefreshInsight = 'refreshOntologyModelInsight',
  FetchContract = 'fetchOntologyModelContract',
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
  /**
   * Keyed by dataset as well as model: one model is measured once per dataset it
   * compiles, and the numbers are not interchangeable between them.
   */
  insight: (id?: string, datasetId?: string) =>
    [OntologyModelApiAction.FetchInsight, id, datasetId] as const,
  contract: (id?: string, version?: number) =>
    [OntologyModelApiAction.FetchContract, id, version] as const,
  /**
   * Model-level, not dataset-level: the draft is one document, and the drift it
   * would leave is a fold over every dataset measured against it.
   */
  changes: (id?: string) => [OntologyModelApiAction.FetchChanges, id] as const,
  /** Dismissals are per dataset: one model judged against two corpora is two scopes. */
  dismissals: (id?: string, datasetId?: string) =>
    [OntologyModelApiAction.FetchDismissals, id, datasetId] as const,
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

/**
 * Reads the stored measurement of one dataset against a model.
 *
 * The fetch is gated on both ids because a measurement is scoped to a pair: the
 * same model has different numbers in every dataset it compiles, so a request
 * without one is not a "default" — it is a different question.
 */
export const useFetchOntologyInsight = (
  id?: string,
  datasetId?: string,
  enabled = true,
) => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IOntologyInsight | null>({
    queryKey: OntologyModelKeys.insight(id, datasetId),
    enabled: !!id && !!datasetId && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getOntologyModelInsight(id!, datasetId!);
      return (data?.data?.insight ?? null) as IOntologyInsight | null;
    },
  });

  return { insight: data ?? null, loading, refetch };
};

/**
 * Reads the draft against the published version.
 *
 * Same two facts the publish dialog gets from its check, but available to anyone
 * who can open the model: the drift panel uses it to show what stage the
 * declaration is in, and that is a reader's question.
 */
export const useFetchOntologyModelChanges = (id?: string, enabled = true) => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IOntologyModelPendingChanges | null>({
    queryKey: OntologyModelKeys.changes(id),
    enabled: !!id && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getOntologyModelChanges(id!);
      return (data?.data ?? null) as IOntologyModelPendingChanges | null;
    },
  });

  return { pending: data ?? null, loading, refetch };
};

export interface IDriftDismissal {
  class_name: string;
  reason: string;
  dismissed_by?: string;
}

/**
 * Findings the operator set aside for one model × dataset.
 *
 * Kept apart from the measurement: a recompile rewrites the insight row and must
 * not resurrect what was already judged.
 */
export const useFetchDriftDismissals = (id?: string, datasetId?: string) => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IDriftDismissal[]>({
    queryKey: OntologyModelKeys.dismissals(id, datasetId),
    enabled: !!id && !!datasetId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listDriftDismissals(id!, datasetId!);
      return (data?.data?.dismissals ?? []) as IDriftDismissal[];
    },
  });

  const rows = data ?? [];
  return {
    dismissals: rows,
    dismissed: new Set(rows.map((row) => row.class_name)),
    loading,
    refetch,
  };
};

/** Records a dismissal; the reason is mandatory, so the caller collects it. */
export const useDismissDriftFinding = () => {
  const client = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.DismissFinding],
    mutationFn: async (input: {
      id: string;
      datasetId: string;
      className: string;
      reason: string;
    }) => {
      const { data } = await dismissDriftFinding(input.id, input.datasetId, {
        class_name: input.className,
        reason: input.reason,
      });
      if (data?.code === 0) {
        await client.invalidateQueries({
          queryKey: OntologyModelKeys.dismissals(input.id, input.datasetId),
        });
        return { ok: true };
      }
      return { ok: false, message: data?.message };
    },
  });

  return { loading, dismiss: mutateAsync };
};

/** Lifts a dismissal so the finding comes back into the list. */
export const useRestoreDriftFinding = () => {
  const client = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.RestoreFinding],
    mutationFn: async (input: {
      id: string;
      datasetId: string;
      className: string;
    }) => {
      const { data } = await restoreDriftFinding(
        input.id,
        input.datasetId,
        input.className,
      );
      if (data?.code === 0) {
        await client.invalidateQueries({
          queryKey: OntologyModelKeys.dismissals(input.id, input.datasetId),
        });
        return { ok: true };
      }
      return { ok: false, message: data?.message };
    },
  });

  return { loading, restore: mutateAsync };
};

/**
 * Measures a dataset now and stores the result.
 *
 * The caller invalidates its own queries: the refresh changes the measurement
 * for this pair and the publish gate for the whole model, and neither key is
 * reachable from here without knowing which screen asked.
 *
 * It never throws. A measurement that fails — no data source, an unbound
 * dataset, a model that does not exist — has to come back as a message the
 * button can show, because a rejected promise from an onClick handler is
 * swallowed and the run looks like it did nothing.
 */
export const useRefreshOntologyInsight = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyModelApiAction.RefreshInsight],
    mutationFn: async ({
      id,
      datasetId,
    }: {
      id: string;
      datasetId: string;
    }): Promise<{
      ok: boolean;
      message?: string;
      insight?: IOntologyInsight;
    }> => {
      try {
        const { data } = await refreshOntologyModelInsight(id, datasetId);
        if (data?.code !== 0) {
          return { ok: false, message: data?.message };
        }
        return { ok: true, insight: data?.data as IOntologyInsight };
      } catch (error) {
        return {
          ok: false,
          message:
            (error as { message?: string })?.message ?? 'measurement failed',
        };
      }
    },
  });

  return { refreshInsight: mutateAsync, loading };
};

/**
 * Renders one model version for export.
 *
 * Fetch-on-open: reading a version is a decision about which version to hand to
 * someone else, and caching it would let a re-publish leave the dialog showing
 * yesterday's prose.
 */
export const useFetchOntologyModelContract = (
  id?: string,
  version?: number,
  enabled = true,
) => {
  const { data, isFetching: loading } = useQuery<IOntologyContractExport>({
    queryKey: OntologyModelKeys.contract(id, version),
    enabled: !!id && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getOntologyModelContract(id!, version);
      return data?.data as IOntologyContractExport;
    },
  });

  return { contract: data, loading };
};
