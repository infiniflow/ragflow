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
