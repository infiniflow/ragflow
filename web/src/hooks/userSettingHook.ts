import { ITenantInfo } from '@/interfaces/database/knowledge';
import { IUserInfo } from '@/interfaces/database/userSetting';
import { useCallback, useEffect } from 'react';
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
