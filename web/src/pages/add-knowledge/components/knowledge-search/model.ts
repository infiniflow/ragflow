import kbService from '@/services/kbService';
import omit from 'lodash/omit';
import { DvaModel } from 'umi';

export interface KSearchModelState {
  loading: boolean;
  data: any[];
  total: number;
  isShowCreateModal: boolean;
  chunk_id: string;
  chunkInfo: any;
  d_list: any[];
  question: string;
  doc_ids: any[];
  pagination: any;
  doc_id: string;
}

const model: DvaModel<KSearchModelState> = {
  namespace: 'kSearchModel',
  state: {
    loading: false,
    data: [],
    total: 0,
    isShowCreateModal: false,
    chunk_id: '',
    chunkInfo: {},
    d_list: [],
    question: '',
    doc_ids: [],
    pagination: { page: 1, size: 30 },
    doc_id: '',
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
      history.listen((location) => {
        console.log(location);
      });
    },
  },
  effects: {
    *getKfList({ payload = {} }, { call, put }) {
      const { data, response } = yield call(
        kbService.get_document_list,
        payload,
      );

      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            d_list: res,
          },
        });
      }
    },
    *chunk_list({ payload = {} }, { call, put, select }) {
      const { question, doc_ids, pagination }: KSearchModelState = yield select(
        (state: any) => state.kSearchModel,
      );
      const { data } = yield call(kbService.retrieval_test, {
        ...payload,
        ...pagination,
        question,
        doc_ids,
        similarity_threshold: 0.1,
      });
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            data: res.chunks,
            total: res.total,
          },
        });
      }
    },
    *switch_chunk({ payload = {} }, { call, put }) {
      const { data } = yield call(
        kbService.switch_chunk,
        omit(payload, ['kb_id']),
      );
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'chunk_list',
          payload: {
            kb_id: payload.kb_id,
          },
        });
      }
    },
    *rm_chunk({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.rm_chunk, {
        chunk_ids: payload.chunk_ids,
      });
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        // TODO: Can be extracted
        yield put({
          type: 'chunk_list',
          payload: {
            kb_id: payload.kb_id,
          },
        });
      }
    },
    *get_chunk({ payload = {} }, { call, put }) {
      const { data, response } = yield call(kbService.get_chunk, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            chunkInfo: res,
          },
        });
      }
    },
    *create_hunk({ payload = {} }, { call, put }) {
      yield put({
        type: 'updateState',
        payload: {
          loading: true,
        },
      });
      let service = kbService.create_chunk;
      if (payload.chunk_id) {
        service = kbService.set_chunk;
      }
      const { data } = yield call(service, payload);
      const { retcode } = data;
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
            isShowCreateModal: false,
          },
        });
      }
    },
  },
};
export default model;
