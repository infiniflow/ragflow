import { ITenantInfo } from '@/interfaces/database/knowledge';
import { ISystemStatus, IUserInfo } from '@/interfaces/database/userSetting';
import userService from '@/services/user-service';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useDispatch, useSelector } from 'umi';

export const useFetchUserInfo = () => {
  const dispatch = useDispatch();
  const fetchUserInfo = useCallback(() => {
    dispatch({ type: 'settingModel/getUserInfo' });
  }, [dispatch]);

  useEffect(() => {
    fetchUserInfo();
  }, [fetchUserInfo]);
};

export const useSelectUserInfo = () => {
  const userInfo: IUserInfo = useSelector(
    (state: any) => state.settingModel.userInfo,
  );

  return userInfo;
};

export const useSelectTenantInfo = () => {
  const tenantInfo: ITenantInfo = useSelector(
    (state: any) => state.settingModel.tenantIfo,
  );

  return tenantInfo;
};

export const useFetchTenantInfo = (isOnMountFetching: boolean = true) => {
  const dispatch = useDispatch();

  const fetchTenantInfo = useCallback(() => {
    dispatch({
      type: 'settingModel/getTenantInfo',
    });
  }, [dispatch]);

  useEffect(() => {
    if (isOnMountFetching) {
      fetchTenantInfo();
    }
  }, [fetchTenantInfo, isOnMountFetching]);

  return fetchTenantInfo;
};

export const useSelectParserList = (): Array<{
  value: string;
  label: string;
}> => {
  const tenantInfo: ITenantInfo = useSelectTenantInfo();

  const parserList = useMemo(() => {
    const parserArray: Array<string> = tenantInfo?.parser_ids.split(',') ?? [];
    return parserArray.map((x) => {
      const arr = x.split(':');
      return { value: arr[0], label: arr[1] };
    });
  }, [tenantInfo]);

  return parserList;
};

export const useSaveSetting = () => {
  const dispatch = useDispatch();

  const saveSetting = useCallback(
    (userInfo: { new_password: string } | Partial<IUserInfo>): number => {
      return dispatch<any>({ type: 'settingModel/setting', payload: userInfo });
    },
    [dispatch],
  );

  return saveSetting;
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
