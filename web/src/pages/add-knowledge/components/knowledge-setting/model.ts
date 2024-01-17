import { message } from 'antd';
import { Effect, Reducer, Subscription } from 'umi'
import kbService from '@/services/kbService';

export interface kSModelState {
  isShowPSwModal: boolean;
  isShowTntModal: boolean;
  loading: boolean;
  tenantIfo: any
}
export interface kSModelType {
  namespace: 'kSModel';
  state: kSModelState;
  effects: {
    createKb: Effect;
    updateKb: Effect;
    getKbDetail: Effect;
  };
  reducers: {
    updateState: Reducer<kSModelState>;
  };
  subscriptions: { setup: Subscription };
}
const Model: kSModelType = {
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
        message.success('创建知识库成功！');
        callback && callback(res.kb_id)
      }
    },
    * updateKb({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.updateKb, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('更新知识库成功！');
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
