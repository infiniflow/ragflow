import { message } from 'antd';
import { addParam } from '@/utils';
import kbService from '@/services/kbService';

const Model = {
  namespace: 'knowledgeModel',
  state: {
    isShowPSwModal: false,
    isShowTntModal: false,
    loading: false,
    data: []
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(location => {
        console.log(location)
      });
    }
  },
  effects: {
    * rmKb({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.rmKb, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        callback && callback()

      }
    },
    *setting({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.setting, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('密码修改成功！');
        callback && callback()
      }
    },
    *getUserInfo({ payload = {} }, { call, put }) {
      const { data, response } = yield call(kbService.user_info, payload);
      const { retcode, data: res, retmsg } = data
      const userInfo = {
        avatar: res.avatar,
        name: res.nickname,
        email: res.email
      };
      localStorage.setItem('userInfo', JSON.stringify(userInfo))
      if (retcode === 0) {
        // localStorage.setItem('userInfo',res.)
      }
    },
    *getList({ payload = {} }, { call, put }) {
      yield put({
        type: 'updateState',
        payload: {
          loading: true
        }
      });
      const { data, response } = yield call(kbService.getList, payload);
      const { retcode, data: res, retmsg } = data
      yield put({
        type: 'updateState',
        payload: {
          loading: false
        }
      });
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            data: res
          }
        });
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
