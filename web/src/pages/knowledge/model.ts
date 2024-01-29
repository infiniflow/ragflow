import kbService from '@/services/kbService';
import { Effect, Reducer } from 'umi';

export interface knowledgeModelState {
  loading: boolean;
  data: any[];
}
export interface knowledgegModelType {
  namespace: 'knowledgeModel';
  state: knowledgeModelState;
  effects: {
    rmKb: Effect;
    getList: Effect;
  };
  reducers: {
    updateState: Reducer<knowledgeModelState>;
  };
  // subscriptions: { setup: Subscription };
}
const Model: knowledgegModelType = {
  namespace: 'knowledgeModel',
  state: {
    loading: false,
    data: [],
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
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
  },
};
export default Model;
