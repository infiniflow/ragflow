import { ITenantInfo } from '@/interfaces/database/knowledge';
import { IUserInfo } from '@/interfaces/database/userSetting';
import authorizationUtil from '@/utils/authorizationUtil';
import { useCallback, useEffect, useMemo } from 'react';
import { history, useDispatch, useSelector } from 'umi';

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

export const useLogout = () => {
  const dispatch = useDispatch(); // TODO: clear redux state

  const logout = useCallback(async () => {
    const retcode = await dispatch<any>({ type: 'loginModel/logout' });
    if (retcode === 0) {
      authorizationUtil.removeAll();
      history.push('/login');
    }
  }, [dispatch]);

  return logout;
};

export const useSaveSetting = () => {
  const dispatch = useDispatch();

  const saveSetting = useCallback(
    (userInfo: { new_password: string } | IUserInfo): number => {
      return dispatch<any>({ type: 'settingModel/setting', payload: userInfo });
    },
    [dispatch],
  );

  return saveSetting;
};
