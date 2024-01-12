import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  login, register, setting, user_info, tenant_info } = api;
interface userServiceType {
  login: (params: any) => void
}
const userService = registerServer(
  {
    login: {
      url: login,
      method: 'post',

    },
    register: {
      url: register,
      method: 'post'
    },
    setting: {
      url: setting,
      method: 'post'
    },
    user_info: {
      url: user_info,
      method: 'get'
    },
    get_tenant_info: {
      url: tenant_info,
      method: 'get'
    },
  },
  request
);

export default userService;
