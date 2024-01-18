import { Effect, Reducer, Subscription } from 'umi'
import { message } from 'antd';
import kbService from '@/services/kbService';

export interface chunkModelState {
  loading: boolean;
  data: any[];
  total: number;
  isShowCreateModal: boolean;
  chunk_id: string;
  chunkInfo: any
}
export interface chunkgModelType {
  namespace: 'chunkModel';
  state: chunkModelState;
  effects: {
    chunk_list: Effect;
    get_chunk: Effect;
    create_hunk: Effect;
    switch_chunk: Effect;
    rm_chunk: Effect;
  };
  reducers: {
    updateState: Reducer<chunkModelState>;
  };
  subscriptions: { setup: Subscription };
}
const Model: chunkgModelType = {
  namespace: 'chunkModel',
  state: {
    loading: false,
    data: [],
    total: 0,
    isShowCreateModal: false,
    chunk_id: '',
    chunkInfo: {}
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(location => {
        console.log(location)
      });
    }
  },
  effects: {
    * chunk_list({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.chunk_list, payload);

      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        console.log(res)
        yield put({
          type: 'updateState',
          payload: {
            data: res.chunks,
            total: res.total,
            loading: false
          }
        });
        callback && callback()

      }
    },
    *switch_chunk({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.switch_chunk, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        callback && callback()

      }
    },
    *rm_chunk({ payload = {}, callback }, { call, put }) {
      console.log('shanchu')
      const { data, response } = yield call(kbService.rm_chunk, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        callback && callback()

      }
    },
    * get_chunk({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.get_chunk, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {

        yield put({
          type: 'updateState',
          payload: {
            chunkInfo: res
          }
        });
        callback && callback(res)

      }
    },
    *create_hunk({ payload = {} }, { call, put }) {
      yield put({
        type: 'updateState',
        payload: {
          loading: true
        }
      });
      let service = kbService.create_chunk
      if (payload.chunk_id) {
        service = kbService.set_chunk
      }
      const { data, response } = yield call(service, payload);
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
            isShowCreateModal: false
          }
        });
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
