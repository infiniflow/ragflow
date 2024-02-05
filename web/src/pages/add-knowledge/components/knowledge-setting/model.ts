import kbService from '@/services/kbService';
import { message } from 'antd';
import { DvaModel } from 'umi';

export interface KSModelState {
  isShowPSwModal: boolean;
  isShowTntModal: boolean;
  tenantIfo: any;
}

const model: DvaModel<KSModelState> = {
  namespace: 'kSModel',
  state: {
    isShowPSwModal: false,
    isShowTntModal: false,
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
    *createKb({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.createKb, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success('创建知识库成功！');
      }
      return data;
    },
    *updateKb({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.updateKb, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('更新知识库成功！');
      }
    },
    *getKbDetail({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.get_kb_detail, payload);

      return data;
    },
  },
};
export default model;
