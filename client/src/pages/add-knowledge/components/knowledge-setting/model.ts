import { message } from 'antd';
import { addParam } from '@/utils';
import kbService from '@/services/kbService';

const Model = {
  namespace: 'kSModel',
  state: {
    isShowPSwModal: false,
    isShowTntModal: false,
    loading: false,
    tenantIfo: {}
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(location => {
      });
    }
  },
  effects: {
    * createKb({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.createKb, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {


      }
    },
    * updateKb({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.updateKb, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {


      }
    },
    *getKbDetail({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.get_kb_detail, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        // localStorage.setItem('userInfo',res.)
        callback && callback(res)
      }
    },
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
