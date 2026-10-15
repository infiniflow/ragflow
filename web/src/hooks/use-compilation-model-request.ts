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
  ICompilationModel,
  ICompilationModelImpact,
  ICompilationModelInspect,
  ICompilationModelInspectParams,
  ICompilationModelPublishCheck,
  ICompilationModelTrial,
  ICompilationModelVersion,
  ICompilationModelVersionDiff,
} from '@/interfaces/database/compilation-model';
import i18n from '@/locales/config';
import {
  checkCompilationModel,
  diffCompilationModelVersions,
  inspectCompilationModelInstances,
  listCompilationModelDatasets,
  listCompilationModelVersions,
  listCompilationModels,
  publishCompilationModel,
  rejectCompilationModel,
  retireCompilationModel,
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
    enabled: !!id && !!(params?.class || params?.property),
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
