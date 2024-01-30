import { Authorization } from '@/constants/authorization';
import userService from '@/services/userService';
import authorizationUtil from '@/utils/authorizationUtil';
import { message } from 'antd';
import { DvaModel } from 'umi';

export interface LoginModelState {
  list: any[];
  info: any;
  visible: boolean;
}

const model: DvaModel<LoginModelState> = {
  namespace: 'loginModel',
  state: {
    list: [],
    info: {},
    visible: false,
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
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
      }
      return data;
    },
    *register({ payload = {} }, { call, put }) {
      const { data, response } = yield call(userService.register, payload);
      console.log();
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('注册成功！');
      }
    },
  },
};
export default model;
