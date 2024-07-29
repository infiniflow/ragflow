import { LanguageTranslationMap } from '@/constants/common';
import { ResponseGetType } from '@/interfaces/database/base';
import { ITenantInfo } from '@/interfaces/database/knowledge';
import { ISystemStatus, IUserInfo } from '@/interfaces/database/userSetting';
import userService from '@/services/user-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { message } from 'antd';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const useFetchUserInfo = (): ResponseGetType<IUserInfo> => {
  const { i18n } = useTranslation();

  const { data, isFetching: loading } = useQuery({
    queryKey: ['userInfo'],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data } = await userService.user_info();
      if (data.retcode === 0) {
        i18n.changeLanguage(
          LanguageTranslationMap[
            data.language as keyof typeof LanguageTranslationMap
          ],
        );
      }
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useFetchTenantInfo = (): ResponseGetType<ITenantInfo> => {
  const { data, isFetching: loading } = useQuery({
    queryKey: ['tenantInfo'],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data: res } = await userService.get_tenant_info();
      if (res.retcode === 0) {
        // llm_id is chat_id
        // asr_id is speech2txt
        const { data } = res;
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
  const { data: tenantInfo } = useFetchTenantInfo();

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
      if (data.retcode === 0) {
        message.success(t('message.modified'));
        queryClient.invalidateQueries({ queryKey: ['userInfo'] });
      }
      return data?.retcode;
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
      if (data.retcode === 0) {
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
    if (data.retcode === 0) {
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
