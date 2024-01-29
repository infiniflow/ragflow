import { Authorization } from '@/constants/authorization';
import userService from '@/services/userService';
import authorizationUtil from '@/utils/authorizationUtil';
import { message } from 'antd';
import { Effect, Reducer, Subscription } from 'umi';

export interface loginModelState {
  list: any[];
  info: any;
  visible: boolean;
}
export interface logingModelType {
  namespace: 'loginModel';
  state: loginModelState;
  effects: {
    login: Effect;
    register: Effect;
  };
  reducers: {
    updateState: Reducer<loginModelState>;
  };
  subscriptions: { setup: Subscription };
}
const Model: logingModelType = {
  namespace: 'loginModel',
  state: {
    list: [],
    info: {},
    visible: false,
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen((location) => {});
    },
  },
  effects: {
    *login({ payload = {} }, { call, put }) {
      console.log(111, payload);
      const { data, response } = yield call(userService.login, payload);
      const { retcode, data: res, retmsg } = data;
      console.log();
      const authorization = response.headers.get(Authorization);
      if (retcode === 0) {
        message.success('登录成功！');
        const token = res.access_token;
        const userInfo = {
          avatar: res.avatar,
          name: res.nickname,
          email: res.email,
        };
        authorizationUtil.setItems({
          Authorization: authorization,
          userInfo: JSON.stringify(userInfo),
          Token: token,
        });
        // setTimeout(() => {
        //   window.location.href = '/file';
        // }, 300);
      }
      return data;
    },
    *register({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(userService.register, payload);
      console.log();
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('注册成功！');
        callback && callback();
      }
    },
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
  },
};
export default Model;
