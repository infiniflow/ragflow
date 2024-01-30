import kbService from '@/services/kbService';
import { message } from 'antd';
import { DvaModel } from 'umi';

export interface KSModelState {
  isShowPSwModal: boolean;
  isShowTntModal: boolean;
  loading: boolean;
  tenantIfo: any;
}

const model: DvaModel<KSModelState> = {
  namespace: 'kSModel',
  state: {
    isShowPSwModal: false,
    isShowTntModal: false,
    loading: false,
    tenantIfo: {},
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
    *createKb({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.createKb, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('创建知识库成功！');
        callback && callback(res.kb_id);
      }
    },
    *updateKb({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.updateKb, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('更新知识库成功！');
      }
    },
    *getKbDetail({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.get_kb_detail, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        // localStorage.setItem('userInfo',res.)
        callback && callback(res);
      }
    },
  },
};
export default model;
