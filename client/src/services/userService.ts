import api from '@/utils/api';
import registerServer from '@/utils/registerServer';
import request from '@/utils/request';

const {
  login, register } = api;
interface userServiceType {
  login: (params: any) => void
}
const userService = registerServer(
  {
    login: {
      url: login,
      method: 'post'
    },
    register: {
      url: register,
      method: 'post'
    }
  },
  request
);

export default userService;
