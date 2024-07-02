import { ResponseType } from '@/interfaces/database/base';
import { DSL, IFlow, IFlowTemplate } from '@/interfaces/database/flow';
import i18n from '@/locales/config';
import flowService from '@/services/flow-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { message } from 'antd';
import { useParams } from 'umi';

export const useFetchFlowTemplates = (): ResponseType<IFlowTemplate[]> => {
  const { data } = useQuery({
    queryKey: ['fetchFlowTemplates'],
    initialData: [],
    queryFn: async () => {
      const { data } = await flowService.listTemplates();

      return data;
    },
  });

  return data;
};

export const useFetchFlowList = (): { data: IFlow[]; loading: boolean } => {
  const { data, isFetching: loading } = useQuery({
    queryKey: ['fetchFlowList'],
    initialData: [],
    queryFn: async () => {
      const { data } = await flowService.listCanvas();

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchFlow = (): { data: IFlow; loading: boolean } => {
  const { id } = useParams();
  const { data, isFetching: loading } = useQuery({
    queryKey: ['flowDetail'],
    initialData: {} as IFlow,
    queryFn: async () => {
      const { data } = await flowService.getCanvas({}, id);

      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useSetFlow = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['setFlow'],
    mutationFn: async (params: { id?: string; title?: string; dsl?: DSL }) => {
      const { data } = await flowService.setCanvas(params);
      if (data.retcode === 0) {
        message.success(
          i18n.t(`message.${params?.id ? 'modified' : 'created'}`),
        );
        queryClient.invalidateQueries({ queryKey: ['fetchFlowList'] });
      }
      return data;
    },
  });

  return { data, loading, setFlow: mutateAsync };
};

export const useDeleteFlow = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteFlow'],
    mutationFn: async (canvasIds: string[]) => {
      const { data } = await flowService.removeCanvas({ canvasIds });
      if (data.retcode === 0) {
        queryClient.invalidateQueries({ queryKey: ['fetchFlowList'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteFlow: mutateAsync };
};

export const useRunFlow = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['runFlow'],
    mutationFn: async (params: { id: string; dsl: DSL }) => {
      const { data } = await flowService.runCanvas(params);
      if (data.retcode === 0) {
        message.success(i18n.t(`message.modified`));
      }
      return data?.data ?? {};
    },
  });

  return { data, loading, runFlow: mutateAsync };
};
