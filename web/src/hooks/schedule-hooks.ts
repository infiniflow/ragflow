import message from '@/components/ui/message';
import {
  ICreateScheduleRequest,
  IFrequencyOptions,
  IUpdateScheduleRequest,
} from '@/interfaces/database/schedule';
import agentService, {
  deleteScheduleById,
  getScheduleHistoryById,
  getScheduleStatsById,
  toggleScheduleById,
} from '@/services/agent-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

export const useFetchFrequencyOptions = () => {
  const { data, isLoading } = useQuery<IFrequencyOptions>({
    queryKey: ['frequencyOptions'],
    queryFn: async () => {
      const { data } = await agentService.getFrequencyOptions();
      return data?.data ?? {};
    },
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
  });

  return { data, loading: isLoading };
};

export const useFetchSchedules = (
  canvas_id = '',
  page = 1,
  pageSize = 20,
  keywords = '',
) => {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['schedules', canvas_id, page, pageSize, keywords],
    queryFn: async () => {
      const { data } = await agentService.listSchedules({
        page,
        page_size: pageSize,
        canvas_id,
        keywords,
      });
      return data?.data ?? { schedules: [], total: 0 };
    },
  });

  return {
    schedules: data?.schedules ?? [],
    total: data?.total ?? 0,
    loading: isLoading,
    refetch,
  };
};

export const useCreateSchedule = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const { mutateAsync, isPending } = useMutation({
    mutationFn: async (params: ICreateScheduleRequest) => {
      const { data } = await agentService.createSchedule(params);
      return data;
    },
    onSuccess: () => {
      message.success(t('flow.schedule.createSuccess'));
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
    },
    onError: (error: any) => {
      message.error(
        error?.response?.data?.message || t('flow.schedule.createError'),
      );
    },
  });

  return { createSchedule: mutateAsync, loading: isPending };
};

export const useUpdateSchedule = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const { mutateAsync, isPending } = useMutation({
    mutationFn: async (params: IUpdateScheduleRequest) => {
      const { data } = await agentService.updateSchedule(params);
      return data;
    },
    onSuccess: () => {
      message.success(t('flow.schedule.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
    },
    onError: (error: any) => {
      console.error('Update schedule error:', error);
      message.error(
        error?.response?.data?.message || t('flow.schedule.updateError'),
      );
    },
  });

  return { updateSchedule: mutateAsync, loading: isPending };
};

export const useToggleSchedule = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const { mutateAsync, isPending } = useMutation({
    mutationFn: async (id: string) => {
      const { data } = await toggleScheduleById({}, id);
      return data;
    },
    onSuccess: () => {
      message.success(t('flow.schedule.toggleSuccess'));
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
    },
    onError: (error: any) => {
      message.error(
        error?.response?.data?.message || t('flow.schedule.toggleError'),
      );
    },
  });

  return { toggleSchedule: mutateAsync, loading: isPending };
};

export const useDeleteSchedule = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const { mutateAsync, isPending } = useMutation({
    mutationFn: async (id: string) => {
      const { data } = await deleteScheduleById({}, id);
      return data;
    },
    onSuccess: () => {
      message.success(t('flow.schedule.deleteSuccess'));
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
    },
    onError: (error: any) => {
      message.error(
        error?.response?.data?.message || t('flow.schedule.deleteError'),
      );
    },
  });

  return { deleteSchedule: mutateAsync, loading: isPending };
};

export const useFetchScheduleHistory = (scheduleId: string, limit = 20) => {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['scheduleHistory', scheduleId, limit],
    queryFn: async () => {
      const { data } = await getScheduleHistoryById(
        {
          limit,
        },
        scheduleId,
      );
      return data?.data ?? [];
    },
    enabled: !!scheduleId,
  });

  return {
    history: data ?? [],
    loading: isLoading,
    refetch,
  };
};

export const useFetchScheduleStats = (scheduleId: string) => {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['scheduleStats', scheduleId],
    queryFn: async () => {
      const { data } = await getScheduleStatsById({}, scheduleId);
      return data?.data ?? {};
    },
    enabled: !!scheduleId,
    refetchInterval: 30000, // Refresh every 30 seconds
  });

  return {
    stats: data ?? {},
    loading: isLoading,
    refetch,
  };
};
