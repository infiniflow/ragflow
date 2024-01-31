import kbService from '@/services/kbService';
import { DvaModel } from 'umi';

export interface KnowledgeModelState {
  data: any[];
}

const model: DvaModel<KnowledgeModelState> = {
  namespace: 'knowledgeModel',
  state: {
    data: [],
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
    *rmKb({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.rmKb, payload);
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'getList',
          payload: {},
        });
      }
    },
    *getList({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.getList, payload);
      const { retcode, data: res, retmsg } = data;

      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            data: res,
          },
        });
      }
    },
  },
};
export default model;
