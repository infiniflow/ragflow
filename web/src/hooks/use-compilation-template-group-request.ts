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

import message from '@/components/ui/message';
import { ListDeletionKey } from '@/constants/list-deletion';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import {
  ICreateCompilationTemplateGroupRequestBody,
  IUpdateCompilationTemplateGroupRequestBody,
} from '@/interfaces/request/compilation-template';
import i18n from '@/locales/config';
import {
  compilationTemplateGroupService,
  createCompilationTemplateGroup,
  deleteCompilationTemplateGroup,
  getCompilationTemplateGroup,
  updateCompilationTemplateGroup,
} from '@/services/compilation-template-group-service';
import { isCreateCompilationTemplateGroup } from '@/utils/compilation-template-util';
import { markListItemsDeleted } from '@/utils/list-deletion-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo } from 'react';
import { useParams } from 'react-router';

import { AgentKeys } from './use-agent-request';

export const enum CompilationTemplateGroupApiAction {
  FetchCompilationTemplateGroups = 'fetchCompilationTemplateGroups',
  FetchCompilationTemplateGroup = 'fetchCompilationTemplateGroup',
  CreateCompilationTemplateGroup = 'createCompilationTemplateGroup',
  UpdateCompilationTemplateGroup = 'updateCompilationTemplateGroup',
  DeleteCompilationTemplateGroup = 'deleteCompilationTemplateGroup',
}

export const CompilationTemplateGroupKeys = {
  detail: (id?: string) =>
    [
      CompilationTemplateGroupApiAction.FetchCompilationTemplateGroup,
      id,
    ] as const,
  all: () =>
    [CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups] as const,
};

export const useFetchCompilationTemplateGroup = () => {
  const { id } = useParams<{ id: string }>();
  const isCreate = isCreateCompilationTemplateGroup(id);

  const { data, isFetching: loading } = useQuery<
    ICompilationTemplateGroup | undefined
  >({
    queryKey: CompilationTemplateGroupKeys.detail(id),
    enabled: !isCreate,
    gcTime: 0,
    queryFn: async () => {
      if (isCreate) return undefined;
      const { data } = await getCompilationTemplateGroup(id);
      return data?.data as ICompilationTemplateGroup | undefined;
    },
  });

  return { data, loading };
};

export const useCreateCompilationTemplateGroup = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [
      CompilationTemplateGroupApiAction.CreateCompilationTemplateGroup,
    ],
    mutationFn: async (params: ICreateCompilationTemplateGroupRequestBody) => {
      const { data } = await createCompilationTemplateGroup(params);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [
          CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups,
        ],
      });
    },
  });

  const createGroup = useCallback(
    async (params: ICreateCompilationTemplateGroupRequestBody) => {
      const result = await mutateAsync(params);
      if (result.code === 0) {
        message.success(i18n.t('message.created'));
      }
      return result;
    },
    [mutateAsync],
  );

  return { data, loading, createGroup };
};

export const useUpdateCompilationTemplateGroup = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [
      CompilationTemplateGroupApiAction.UpdateCompilationTemplateGroup,
    ],
    mutationFn: async ({
      id,
      params,
    }: {
      id: string;
      params: IUpdateCompilationTemplateGroupRequestBody;
    }) => {
      const { data } = await updateCompilationTemplateGroup(id, params);
      return data;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: [
          CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups,
        ],
      });
      queryClient.invalidateQueries({
        queryKey: CompilationTemplateGroupKeys.detail(variables.id),
      });
    },
  });

  const updateGroup = useCallback(
    async (id: string, params: IUpdateCompilationTemplateGroupRequestBody) => {
      const result = await mutateAsync({ id, params });
      if (result.code === 0) {
        message.success(i18n.t('message.modified'));
      }
      return result;
    },
    [mutateAsync],
  );

  return { data, loading, updateGroup };
};

export const useDeleteCompilationTemplateGroup = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [
      CompilationTemplateGroupApiAction.DeleteCompilationTemplateGroup,
    ],
    mutationFn: async (id: string) => {
      const { data } = await deleteCompilationTemplateGroup(id);
      if (data.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: [
            CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups,
          ],
        });
        // The agents page lists groups merged into /agents results.
        queryClient.invalidateQueries({
          queryKey: AgentKeys.list(),
        });
        queryClient.invalidateQueries({
          queryKey: AgentKeys.filters(),
        });
        queryClient.invalidateQueries({
          queryKey: AgentKeys.tags(),
        });
        markListItemsDeleted(ListDeletionKey.AgentList);
      }
      return data?.data ?? true;
    },
  });

  const deleteGroup = useCallback(
    async (id: string) => {
      await mutateAsync(id);
    },
    [mutateAsync],
  );

  return { data, loading, deleteGroup };
};

export const useFetchAllCompilationTemplateGroups = () => {
  const { data, isFetching: loading } = useQuery<ICompilationTemplateGroup[]>({
    queryKey: CompilationTemplateGroupKeys.all(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await compilationTemplateGroupService.listGroups(
        {
          params: { keywords: '', page: 1, page_size: 100 },
        },
        true,
      );
      return (data?.data?.groups ?? []) as ICompilationTemplateGroup[];
    },
  });

  return { groups: data ?? [], loading };
};

export const useCompilationTemplateGroupOptions = () => {
  const { groups } = useFetchAllCompilationTemplateGroups();

  return useMemo(
    () => groups.map((group) => ({ label: group.name, value: group.id })),
    [groups],
  );
};
