import kbService from '@/services/kbService';
import { DvaModel } from 'umi';

export interface KnowledgeModelState {
  loading: boolean;
  data: any[];
}

const model: DvaModel<KnowledgeModelState> = {
  namespace: 'knowledgeModel',
  state: {
    loading: false,
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
  // subscriptions: {
  //   setup({ dispatch, history }) {
  //     history.listen((location) => {
  //       console.log(location);
  //     });
  //   },
  // },
  effects: {
    *rmKb({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.rmKb, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        callback && callback();
      }
    },
    *getList({ payload = {} }, { call, put }) {
      yield put({
        type: 'updateState',
        payload: {
          loading: true,
        },
      });
      const { data, response } = yield call(kbService.getList, payload);
      const { retcode, data: res, retmsg } = data;
      yield put({
        type: 'updateState',
        payload: {
          loading: false,
        },
      });
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
