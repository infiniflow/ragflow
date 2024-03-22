import { IKnowledge } from '@/interfaces/database/knowledge';
import kbService from '@/services/kbService';
import { message } from 'antd';
import { DvaModel } from 'umi';

export interface KSModelState {
  isShowPSwModal: boolean;
  tenantIfo: any;
  knowledgeDetails: IKnowledge;
}

const model: DvaModel<KSModelState> = {
  namespace: 'kSModel',
  state: {
    isShowPSwModal: false,
    tenantIfo: {},
    knowledgeDetails: {} as any,
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
    setKnowledgeDetails(state, { payload }) {
      return { ...state, knowledgeDetails: payload };
    },
  },
  effects: {
    *createKb({ payload = {} }, { call }) {
      const { data } = yield call(kbService.createKb, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success('Created!');
      }
      return data;
    },
    *updateKb({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.updateKb, payload);
      const { retcode } = data;
      if (retcode === 0) {
        yield put({ type: 'getKbDetail', payload: { kb_id: payload.kb_id } });
        message.success('Updated!');
      }
    },
    *getKbDetail({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.get_kb_detail, payload);
      if (data.retcode === 0) {
        yield put({ type: 'setKnowledgeDetails', payload: data.data });
      }
      return data;
    },
  },
};
export default model;
