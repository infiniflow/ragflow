import { LanguageTranslationMap } from '@/constants/common';
import { ResponseGetType } from '@/interfaces/database/base';
import { IToken } from '@/interfaces/database/chat';
import { ITenantInfo } from '@/interfaces/database/knowledge';
import { ILangfuseConfig } from '@/interfaces/database/system';
import {
  ISystemStatus,
  ITenant,
  ITenantUser,
  IUserInfo,
} from '@/interfaces/database/user-setting';
import { ISetLangfuseConfigRequestBody } from '@/interfaces/request/system';
import userService, {
  addTenantUser,
  agreeTenant,
  deleteTenantUser,
  listTenant,
  listTenantUser,
} from '@/services/user-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Modal, message } from 'antd';
import DOMPurify from 'dompurify';
import { isEmpty } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { history } from 'umi';

export const useFetchUserInfo = (): ResponseGetType<IUserInfo> => {
  const { i18n } = useTranslation();

  const { data, isFetching: loading } = useQuery({
    queryKey: ['userInfo'],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data } = await userService.user_info();
      if (data.code === 0) {
        i18n.changeLanguage(
          LanguageTranslationMap[
            data.data.language as keyof typeof LanguageTranslationMap
          ],
        );
      }
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useFetchTenantInfo = (
  showEmptyModelWarn = false,
): ResponseGetType<ITenantInfo> => {
  const { t } = useTranslation();
  const { data, isFetching: loading } = useQuery({
    queryKey: ['tenantInfo'],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data: res } = await userService.get_tenant_info();
      if (res.code === 0) {
        // llm_id is chat_id
        // asr_id is speech2txt
        const { data } = res;
        if (
          showEmptyModelWarn &&
          (isEmpty(data.embd_id) || isEmpty(data.llm_id))
        ) {
          Modal.warning({
            title: t('common.warn'),
            content: (
              <div
                dangerouslySetInnerHTML={{
                  __html: DOMPurify.sanitize(t('setting.modelProvidersWarn')),
                }}
              ></div>
            ),
            onOk() {
              history.push('/user-setting/model');
            },
          });
        }
        data.chat_id = data.llm_id;
        data.speech2text_id = data.asr_id;

        return data;
      }

      return res;
    },
  });

  return { data, loading };
};

export const useSelectParserList = (): Array<{
  value: string;
  label: string;
}> => {
  const { data: tenantInfo } = useFetchTenantInfo(true);

  const parserList = useMemo(() => {
    const parserArray: Array<string> = tenantInfo?.parser_ids?.split(',') ?? [];
    return parserArray.map((x) => {
      const arr = x.split(':');
      return { value: arr[0], label: arr[1] };
    });
  }, [tenantInfo]);

  return parserList;
};

export const useSaveSetting = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['saveSetting'],
    mutationFn: async (
      userInfo: { new_password: string } | Partial<IUserInfo>,
    ) => {
      const { data } = await userService.setting(userInfo);
      if (data.code === 0) {
        message.success(t('message.modified'));
        queryClient.invalidateQueries({ queryKey: ['userInfo'] });
      }
      return data?.code;
    },
  });

  return { data, loading, saveSetting: mutateAsync };
};

export const useFetchSystemVersion = () => {
  const [version, setVersion] = useState('');
  const [loading, setLoading] = useState(false);

  const fetchSystemVersion = useCallback(async () => {
    try {
      setLoading(true);
      const { data } = await userService.getSystemVersion();
      if (data.code === 0) {
        setVersion(data.data);
        setLoading(false);
      }
    } catch (error) {
      setLoading(false);
    }
  }, []);

  return { fetchSystemVersion, version, loading };
};

export const useFetchSystemStatus = () => {
  const [systemStatus, setSystemStatus] = useState<ISystemStatus>(
    {} as ISystemStatus,
  );
  const [loading, setLoading] = useState(false);

  const fetchSystemStatus = useCallback(async () => {
    setLoading(true);
    const { data } = await userService.getSystemStatus();
    if (data.code === 0) {
      setSystemStatus(data.data);
      setLoading(false);
    }
  }, []);

  return {
    systemStatus,
    fetchSystemStatus,
    loading,
  };
};

export const useFetchManualSystemTokenList = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['fetchManualSystemTokenList'],
    mutationFn: async () => {
      const { data } = await userService.listToken();

      return data?.data ?? [];
    },
  });

  return { data, loading, fetchSystemTokenList: mutateAsync };
};

export const useFetchSystemTokenList = () => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IToken[]>({
    queryKey: ['fetchSystemTokenList'],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await userService.listToken();

      return data?.data ?? [];
    },
  });

  return { data, loading, refetch };
};

export const useRemoveSystemToken = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['removeSystemToken'],
    mutationFn: async (token: string) => {
      const { data } = await userService.removeToken({}, token);
      if (data.code === 0) {
        message.success(t('message.deleted'));
        queryClient.invalidateQueries({ queryKey: ['fetchSystemTokenList'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, removeToken: mutateAsync };
};

export const useCreateSystemToken = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createSystemToken'],
    mutationFn: async (params: Record<string, any>) => {
      const { data } = await userService.createToken(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: ['fetchSystemTokenList'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, createToken: mutateAsync };
};

export const useListTenantUser = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const tenantId = tenantInfo.tenant_id;
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<ITenantUser[]>({
    queryKey: ['listTenantUser', tenantId],
    initialData: [],
    gcTime: 0,
    enabled: !!tenantId,
    queryFn: async () => {
      const { data } = await listTenantUser(tenantId);

      return data?.data ?? [];
    },
  });

  return { data, loading, refetch };
};

export const useAddTenantUser = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['addTenantUser'],
    mutationFn: async (email: string) => {
      const { data } = await addTenantUser(tenantInfo.tenant_id, email);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: ['listTenantUser'] });
      }
      return data?.code;
    },
  });

  return { data, loading, addTenantUser: mutateAsync };
};

export const useDeleteTenantUser = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteTenantUser'],
    mutationFn: async ({
      userId,
      tenantId,
    }: {
      userId: string;
      tenantId?: string;
    }) => {
      const { data } = await deleteTenantUser({
        tenantId: tenantId ?? tenantInfo.tenant_id,
        userId,
      });
      if (data.code === 0) {
        message.success(t('message.deleted'));
        queryClient.invalidateQueries({ queryKey: ['listTenantUser'] });
        queryClient.invalidateQueries({ queryKey: ['listTenant'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteTenantUser: mutateAsync };
};

export const useListTenant = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const tenantId = tenantInfo.tenant_id;
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<ITenant[]>({
    queryKey: ['listTenant', tenantId],
    initialData: [],
    gcTime: 0,
    enabled: !!tenantId,
    queryFn: async () => {
      const { data } = await listTenant();

      return data?.data ?? [];
    },
  });

  return { data, loading, refetch };
};

export const useAgreeTenant = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['agreeTenant'],
    mutationFn: async (tenantId: string) => {
      const { data } = await agreeTenant(tenantId);
      if (data.code === 0) {
        message.success(t('message.operated'));
        queryClient.invalidateQueries({ queryKey: ['listTenant'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, agreeTenant: mutateAsync };
};

export const useSetLangfuseConfig = () => {
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['setLangfuseConfig'],
    mutationFn: async (params: ISetLangfuseConfigRequestBody) => {
      const { data } = await userService.setLangfuseConfig(params);
      if (data.code === 0) {
        message.success(t('message.operated'));
      }
      return data?.code;
    },
  });

  return { data, loading, setLangfuseConfig: mutateAsync };
};

export const useDeleteLangfuseConfig = () => {
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteLangfuseConfig'],
    mutationFn: async () => {
      const { data } = await userService.deleteLangfuseConfig();
      if (data.code === 0) {
        message.success(t('message.deleted'));
      }
      return data?.code;
    },
  });

  return { data, loading, deleteLangfuseConfig: mutateAsync };
};

export const useFetchLangfuseConfig = () => {
  const { data, isFetching: loading } = useQuery<ILangfuseConfig>({
    queryKey: ['fetchLangfuseConfig'],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await userService.getLangfuseConfig();

      return data?.data;
    },
  });

  return { data, loading };
};

// Department Hooks
export const useListDepartment = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const tenantId = tenantInfo.tenant_id;
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery({
    queryKey: ['listDepartment', tenantId],
    initialData: [],
    gcTime: 0,
    enabled: !!tenantId,
    queryFn: async () => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.listDepartment(tenantId);
      // return data?.data ?? [];
      
      // 模拟数据，后端实现后移除
      return [
        { id: '1', name: '研发部', description: '负责产品研发', member_count: 5, create_date: '2025-04-01T00:00:00Z' },
        { id: '2', name: '市场部', description: '负责市场营销', member_count: 3, create_date: '2025-04-02T00:00:00Z' },
        { id: '3', name: '运营部', description: '负责产品运营', member_count: 4, create_date: '2025-04-03T00:00:00Z' },
      ];
    },
  });

  return { data, loading, refetch };
};

export const useAddDepartment = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['addDepartment'],
    mutationFn: async (params: { name: string; description?: string; parentId?: string }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.addDepartment(tenantInfo.tenant_id, params);
      // if (data.code === 0) {
      //   queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.created'));
      queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      return 0;
    },
  });

  return { data, loading, addDepartment: mutateAsync };
};

export const useUpdateDepartment = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['updateDepartment'],
    mutationFn: async (params: { id: string; name?: string; description?: string; parentId?: string }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.updateDepartment(params);
      // if (data.code === 0) {
      //   queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.updated'));
      queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      return 0;
    },
  });

  return { data, loading, updateDepartment: mutateAsync };
};

export const useDeleteDepartment = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteDepartment'],
    mutationFn: async (departmentId: string) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.deleteDepartment(departmentId);
      // if (data.code === 0) {
      //   message.success(t('message.deleted'));
      //   queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.deleted'));
      queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      return 0;
    },
  });

  return { data, loading, deleteDepartment: mutateAsync };
};

export const useAddUserToDepartment = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['addUserToDepartment'],
    mutationFn: async (params: { departmentId: string; userIds: string[] }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.addUserToDepartment(params);
      // if (data.code === 0) {
      //   message.success(t('message.operated'));
      //   queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.operated'));
      queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      return 0;
    },
  });

  return { data, loading, addUserToDepartment: mutateAsync };
};

export const useRemoveUserFromDepartment = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['removeUserFromDepartment'],
    mutationFn: async (params: { departmentId: string; userId: string }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.removeUserFromDepartment(params);
      // if (data.code === 0) {
      //   message.success(t('message.operated'));
      //   queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.operated'));
      queryClient.invalidateQueries({ queryKey: ['listDepartment'] });
      return 0;
    },
  });

  return { data, loading, removeUserFromDepartment: mutateAsync };
};

// Group Hooks
export const useListGroup = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const tenantId = tenantInfo.tenant_id;
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery({
    queryKey: ['listGroup', tenantId],
    initialData: [],
    gcTime: 0,
    enabled: !!tenantId,
    queryFn: async () => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.listGroup(tenantId);
      // return data?.data ?? [];
      
      // 模拟数据，后端实现后移除
      return [
        { id: '1', name: 'AI研发小组', description: '负责AI算法研发', member_count: 3, create_date: '2025-04-01T00:00:00Z' },
        { id: '2', name: '前端开发小组', description: '负责前端开发', member_count: 4, create_date: '2025-04-02T00:00:00Z' },
        { id: '3', name: '后端开发小组', description: '负责后端开发', member_count: 5, create_date: '2025-04-03T00:00:00Z' },
      ];
    },
  });

  return { data, loading, refetch };
};

export const useAddGroup = () => {
  const { data: tenantInfo } = useFetchTenantInfo();
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['addGroup'],
    mutationFn: async (params: { name: string; description?: string }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.addGroup(tenantInfo.tenant_id, params);
      // if (data.code === 0) {
      //   queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.created'));
      queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      return 0;
    },
  });

  return { data, loading, addGroup: mutateAsync };
};

export const useUpdateGroup = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['updateGroup'],
    mutationFn: async (params: { id: string; name?: string; description?: string }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.updateGroup(params);
      // if (data.code === 0) {
      //   queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.updated'));
      queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      return 0;
    },
  });

  return { data, loading, updateGroup: mutateAsync };
};

export const useDeleteGroup = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteGroup'],
    mutationFn: async (groupId: string) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.deleteGroup(groupId);
      // if (data.code === 0) {
      //   message.success(t('message.deleted'));
      //   queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.deleted'));
      queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      return 0;
    },
  });

  return { data, loading, deleteGroup: mutateAsync };
};

export const useAddUserToGroup = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['addUserToGroup'],
    mutationFn: async (params: { groupId: string; userIds: string[] }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.addUserToGroup(params);
      // if (data.code === 0) {
      //   message.success(t('message.operated'));
      //   queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.operated'));
      queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      return 0;
    },
  });

  return { data, loading, addUserToGroup: mutateAsync };
};

export const useRemoveUserFromGroup = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['removeUserFromGroup'],
    mutationFn: async (params: { groupId: string; userId: string }) => {
      // 后端实现后替换为实际API调用
      // const { data } = await userService.removeUserFromGroup(params);
      // if (data.code === 0) {
      //   message.success(t('message.operated'));
      //   queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      // }
      // return data?.code;
      
      // 模拟成功响应，后端实现后移除
      message.success(t('message.operated'));
      queryClient.invalidateQueries({ queryKey: ['listGroup'] });
      return 0;
    },
  });

  return { data, loading, removeUserFromGroup: mutateAsync };
};
