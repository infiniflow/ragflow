import { message } from 'antd';
import { addParam } from '@/utils';
import userService from '@/services/userService';

const Model = {
  namespace: 'loginModel',
  state: {
    list: [],
    info: {},
    visible: false,
    pagination: {},
    campaignInfo: {}
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(location => { });
    }
  },
  effects: {
    *login({ payload = {} }, { call, put }) {
      console.log(111, payload)
      const { data, response } = yield call(userService.login, payload);
      const { retcode, data: res, retmsg } = data
      console.log()
      const Authorization = response.headers.get('Authorization')
      if (retcode === 0) {
        message.success('登录成功！');
        const token = res.access_token;
        const userInfo = {
          avatar: res.avatar,
          name: res.nickname,
          email: res.email
        };
        localStorage.setItem('token', token)
        localStorage.setItem('userInfo', JSON.stringify(userInfo))
        localStorage.setItem('Authorization', Authorization)
        setTimeout(() => {
          window.location.href = '/file';
        }, 300);
      }
    },
    *register({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(userService.register, payload);
      console.log()
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('注册成功！');
        callback && callback()
      }
    }
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload
      };
    }
  }
};
export default Model;
