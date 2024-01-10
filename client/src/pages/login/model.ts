import { message } from 'antd';
import { addParam } from '@/utils';
import loginService from '@/services/loginService';

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
      console.log(111)
      const { code, data, errorMessage } = yield call(loginService.login, payload);
      setTimeout(() => {
        window.location.href = '/';
      }, 300);
      // if (code === 0) {
      //   message.success('登录成功！');
      //   const name = data.name;
      //   const token = data.token;
      //   const role = data.role;
      //   const title = data.title;
      //   const userInfo = {
      //     role: data.role,
      //     title: data.title,
      //     name: data.name || data.Name,
      //   };
      //   store.token = token;
      //   store.userInfo = userInfo;
      //   setTimeout(() => {
      //     window.location.href = '/file';
      //   }, 300);
      // }
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
