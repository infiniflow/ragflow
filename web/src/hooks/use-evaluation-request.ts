import message from '@/components/ui/message';
import {
  IEvaluationCase,
  IEvaluationDataset,
  IEvaluationRecommendation,
  IEvaluationResult,
  IEvaluationRun,
} from '@/interfaces/database/evaluation';
import evaluationService from '@/services/evaluation-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

export const EvaluationQueryKey = {
  Datasets: 'evaluationDatasets',
  Dataset: 'evaluationDataset',
  Cases: 'evaluationCases',
  Runs: 'evaluationRuns',
  Run: 'evaluationRun',
  Results: 'evaluationResults',
  Recommendations: 'evaluationRecommendations',
} as const;

export const useFetchEvaluationDatasets = (page = 1, pageSize = 20) => {
  return useQuery({
    queryKey: [EvaluationQueryKey.Datasets, page, pageSize],
    queryFn: async () => {
      const { data: response } = await evaluationService.listEvaluationDatasets({
        params: { page, page_size: pageSize },
      });
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as { total: number; datasets: IEvaluationDataset[] };
    },
  });
};

export const useFetchEvaluationDataset = (datasetId?: string) => {
  return useQuery({
    queryKey: [EvaluationQueryKey.Dataset, datasetId],
    enabled: !!datasetId,
    queryFn: async () => {
      const { data: response } = await evaluationService.getEvaluationDataset(
        datasetId,
      );
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as IEvaluationDataset;
    },
  });
};

export const useCreateEvaluationDataset = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      name: string;
      description?: string;
      kb_ids: string[];
    }) => {
      const { data: response } =
        await evaluationService.createEvaluationDataset(body);
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as { id: string };
    },
    onSuccess: () => {
      message.success(t('message.created'));
      queryClient.invalidateQueries({ queryKey: [EvaluationQueryKey.Datasets] });
    },
  });
};

export const useDeleteEvaluationDataset = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (datasetId: string) => {
      const { data: response } =
        await evaluationService.deleteEvaluationDataset(datasetId);
      if (response.code !== 0) {
        throw new Error(response.message);
      }
    },
    onSuccess: () => {
      message.success(t('message.deleted'));
      queryClient.invalidateQueries({ queryKey: [EvaluationQueryKey.Datasets] });
    },
  });
};

export const useFetchEvaluationCases = (datasetId?: string) => {
  return useQuery({
    queryKey: [EvaluationQueryKey.Cases, datasetId],
    enabled: !!datasetId,
    queryFn: async () => {
      const { data: response } = await evaluationService.listEvaluationCases(
        datasetId,
      );
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as { cases: IEvaluationCase[]; total: number };
    },
  });
};

export const useAddEvaluationCase = (datasetId: string) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      question: string;
      reference_answer?: string;
      relevant_chunk_ids?: string[];
    }) => {
      const { data: response } = await evaluationService.addEvaluationCase({
        datasetId,
        ...body,
      });
      if (response.code !== 0) {
        throw new Error(response.message);
      }
    },
    onSuccess: () => {
      message.success(t('message.created'));
      queryClient.invalidateQueries({
        queryKey: [EvaluationQueryKey.Cases, datasetId],
      });
    },
  });
};

export const useDeleteEvaluationCase = (datasetId: string) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (caseId: string) => {
      const { data: response } = await evaluationService.deleteEvaluationCase({
        datasetId,
        caseId,
      });
      if (response.code !== 0) {
        throw new Error(response.message);
      }
    },
    onSuccess: () => {
      message.success(t('message.deleted'));
      queryClient.invalidateQueries({
        queryKey: [EvaluationQueryKey.Cases, datasetId],
      });
    },
  });
};

export const useImportEvaluationCases = (datasetId: string) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      cases: Array<{
        question: string;
        reference_answer?: string;
        relevant_chunk_ids?: string[];
      }>,
    ) => {
      const { data: response } = await evaluationService.importEvaluationCases({
        datasetId,
        cases,
      });
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as { success_count: number; failure_count: number };
    },
    onSuccess: (data: { success_count: number; failure_count: number }) => {
      message.success(
        t('evaluation.importSuccess', {
          count: data?.success_count ?? 0,
        }),
      );
      queryClient.invalidateQueries({
        queryKey: [EvaluationQueryKey.Cases, datasetId],
      });
    },
  });
};

export const useFetchEvaluationRuns = (datasetId?: string) => {
  return useQuery({
    queryKey: [EvaluationQueryKey.Runs, datasetId],
    enabled: !!datasetId,
    refetchInterval: (query: {
      state: { data?: { runs?: IEvaluationRun[] } };
    }) => {
      const runs = query.state.data?.runs;
      if (runs?.some((r) => r.status === 'RUNNING')) {
        return 3000;
      }
      return false;
    },
    queryFn: async () => {
      const { data: response } = await evaluationService.listEvaluationRuns(
        datasetId,
      );
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as { runs: IEvaluationRun[]; total: number };
    },
  });
};

export const useStartEvaluationRun = (datasetId: string) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: { dialog_id: string; name?: string }) => {
      const { data: response } = await evaluationService.startEvaluationRun({
        datasetId,
        dialog_id: body.dialog_id,
        name: body.name,
      });
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as { id: string };
    },
    onSuccess: () => {
      message.success(t('evaluation.runStarted'));
      queryClient.invalidateQueries({
        queryKey: [EvaluationQueryKey.Runs, datasetId],
      });
    },
  });
};

export const useFetchEvaluationRunResults = (
  runId?: string,
  runStatus?: string,
) => {
  return useQuery({
    queryKey: [EvaluationQueryKey.Results, runId],
    enabled: !!runId,
    refetchInterval: runStatus === 'RUNNING' ? 3000 : false,
    queryFn: async () => {
      const { data: response } = await evaluationService.getEvaluationRunResults(
        runId,
      );
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as {
        run: IEvaluationRun;
        results: IEvaluationResult[];
      };
    },
  });
};

export const useFetchEvaluationRecommendations = (
  runId?: string,
  runStatus?: string,
) => {
  return useQuery({
    queryKey: [EvaluationQueryKey.Recommendations, runId],
    enabled: !!runId,
    refetchInterval: runStatus === 'RUNNING' ? 3000 : false,
    queryFn: async () => {
      const { data: response } =
        await evaluationService.getEvaluationRecommendations(runId);
      if (response.code !== 0) {
        throw new Error(response.message);
      }
      return response.data as {
        recommendations: IEvaluationRecommendation[];
      };
    },
  });
};
