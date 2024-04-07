import { Authorization } from '@/constants/authorization';
import i18n from '@/locales/config';
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
  effects: {
    *login({ payload = {} }, { call }) {
      const { data, response } = yield call(userService.login, payload);
      const { retcode, data: res } = data;
      const authorization = response.headers.get(Authorization);
      if (retcode === 0) {
        message.success(i18n.t('message.logged'));
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
      return retcode;
    },
    *register({ payload = {} }, { call }) {
      const { data } = yield call(userService.register, payload);
      console.log();
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.registered'));
      }
      return retcode;
    },
    *logout({ payload = {} }, { call }) {
      const { data } = yield call(userService.logout, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.logout'));
      }
      return retcode;
    },
  },
};
export default model;
