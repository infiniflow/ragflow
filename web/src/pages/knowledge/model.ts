import { IKnowledge } from '@/interfaces/database/knowledge';
import kbService from '@/services/kbService';
import { DvaModel } from 'umi';

export interface KnowledgeModelState {
  data: any[];
  knowledge: IKnowledge;
}

const model: DvaModel<KnowledgeModelState> = {
  namespace: 'knowledgeModel',
  state: {
    data: [],
    knowledge: {} as IKnowledge,
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
    setKnowledge(state, { payload }) {
      return {
        ...state,
        knowledge: payload,
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
      const { retcode, data: res } = data;

      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            data: res,
          },
        });
      }
    },
    *getKnowledgeDetail({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.get_kb_detail, payload);
      if (data.retcode === 0) {
        yield put({ type: 'setKnowledge', payload: data.data });
      }
      return data.retcode;
    },
  },
};
export default model;
