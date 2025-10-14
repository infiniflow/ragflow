import { Authorization } from '@/constants/authorization';
import userService from '@/services/user-service';
import authorizationUtil from '@/utils/authorization-util';
import { useMutation } from '@tanstack/react-query';

// 固定的自动登录凭据
const AUTO_LOGIN_CREDENTIALS = {
  email: 'admin@qq.com',
  password:
    'pEaWGL3CsmL1ftGmOJCJUZ7frVS6sUMzZvy4PHYnD7ATbKwIChofBU201Vj7o2j4oEbZ7rUJt8Y6J5aQVjIjk1gj30gqV17FZ6ftAuIOQkUcTXuc7Axdu3nyGibZTiLw5Hqva9B8ExvPRpQRJMHqaZYmeDPLIUe1xfiWMqK4PbCLjW2MYEazx1hb61Ozk9sV5q97HR3XvQbW44Sm5DMfM3KmhzfHHz1H8HHvmUAwxTxRwpHvewvdUoVEbou6fIOUTITP+XePIEDEoX4y3GnnvtrpxAUGQL8b63ahzogrliBEH7xAz4PjNan7ny5fwVhdNrm6IoT8gqDW3UPkcc8BcQ==',
};

export const useAutoLogin = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: ['autoLogin'],
    mutationFn: async () => {
      try {
        const { data: res = {}, response } = await userService.login(
          AUTO_LOGIN_CREDENTIALS,
        );

        if (res.code === 0) {
          const { data } = res;
          const authorization = response.headers.get(Authorization);
          const token = data.access_token;
          const userInfo = {
            avatar: data.avatar,
            name: data.nickname,
            email: data.email,
          };

          // 静默设置认证信息，不显示任何提醒
          authorizationUtil.setItems({
            Authorization: authorization,
            userInfo: JSON.stringify(userInfo),
            Token: token,
          });

          return true; // 登录成功
        }

        return false; // 登录失败
      } catch (error) {
        console.error('Auto login failed:', error);
        return false;
      }
    },
  });

  return {
    autoLogin: mutateAsync,
    loading,
  };
};
