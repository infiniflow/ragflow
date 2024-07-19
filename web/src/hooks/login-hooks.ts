import { useCallback } from 'react';
import { useDispatch } from 'umi';

export interface ILoginRequestBody {
  email: string;
  password: string;
}

export interface IRegisterRequestBody extends ILoginRequestBody {
  nickname: string;
}

export const useLogin = () => {
  const dispatch = useDispatch();

  const login = useCallback(
    (requestBody: ILoginRequestBody) => {
      // TODO: Type needs to be improved
      return dispatch<any>({
        type: 'loginModel/login',
        payload: requestBody,
      });
    },
    [dispatch],
  );

  return login;
};

export const useRegister = () => {
  const dispatch = useDispatch();

  const register = useCallback(
    (requestBody: IRegisterRequestBody) => {
      return dispatch<any>({
        type: 'loginModel/register',
        payload: requestBody,
      });
    },
    [dispatch],
  );

  return register;
};
