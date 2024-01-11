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
      const { retcode, data, retmsg } = yield call(userService.login, payload);
      if (retcode === 0) {
        message.success('登录成功！');
        const name = data.name;
        const token = data.access_token;
        const role = data.role;
        const title = data.title;
        const userInfo = {
          role: data.avatar,
          title: data.title,
          name: data.nickname,
        };
        localStorage.setItem('token', token)
        localStorage.setItem('userInfo', JSON.stringify(userInfo))
        // setTimeout(() => {
        //   window.location.href = '/file';
        // }, 300);
      }
    },
    *register({ payload = {} }, { call, put }) {
      console.log(111)
      const { retcode, data, retmsg } = yield call(userService.register, payload);
      // setTimeout(() => {
      //   window.location.href = '/';
      // }, 300);
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
