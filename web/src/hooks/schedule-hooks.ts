import {
  ICreateScheduleRequest,
  IFrequencyOptions,
  IUpdateScheduleRequest,
} from '@/interfaces/database/schedule';
import flowService from '@/services/flow-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { message } from 'antd';
import { useTranslation } from 'react-i18next';

export const useFetchFrequencyOptions = () => {
  const { data, isLoading } = useQuery<IFrequencyOptions>({
    queryKey: ['frequencyOptions'],
    queryFn: async () => {
      const { data } = await flowService.getFrequencyOptions();
      return data?.data ?? {};
    },
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
  });

  return { data, loading: isLoading };
};

export const useFetchSchedules = (page = 1, pageSize = 20, keywords = '') => {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['schedules', page, pageSize, keywords],
    queryFn: async () => {
      const { data } = await flowService.listSchedules({
        page,
        page_size: pageSize,
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
      const { data } = await flowService.createSchedule(params);
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
    mutationFn: async ({
      id,
      ...params
    }: IUpdateScheduleRequest & { id: string }) => {
      const { data } = await flowService.updateSchedule(params, id);
      return data;
    },
    onSuccess: () => {
      message.success(t('flow.schedule.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['schedules'] });
    },
    onError: (error: any) => {
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
      const { data } = await flowService.toggleSchedule({}, id);
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
      const { data } = await flowService.deleteSchedule({}, id);
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
