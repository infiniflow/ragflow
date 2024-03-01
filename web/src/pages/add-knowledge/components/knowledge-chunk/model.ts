import { BaseState } from '@/interfaces/common';
import { IChunk, IKnowledgeFile } from '@/interfaces/database/knowledge';
import kbService from '@/services/kbService';
import { message } from 'antd';
import { pick } from 'lodash';
// import { delay } from '@/utils/storeUtil';
import { DvaModel } from 'umi';

export interface ChunkModelState extends BaseState {
  data: IChunk[];
  total: number;
  isShowCreateModal: boolean;
  chunk_id: string;
  doc_id: string;
  chunkInfo: any;
  documentInfo: IKnowledgeFile;
  available?: number;
}

const model: DvaModel<ChunkModelState> = {
  namespace: 'chunkModel',
  state: {
    data: [],
    total: 0,
    isShowCreateModal: false,
    chunk_id: '',
    doc_id: '',
    chunkInfo: {},
    documentInfo: {} as IKnowledgeFile,
    pagination: {
      current: 1,
      pageSize: 10,
    },
    searchString: '',
    available: undefined, // set to undefined to select all
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
    setIsShowCreateModal(state, { payload }) {
      return {
        ...state,
        isShowCreateModal:
          typeof payload === 'boolean' ? payload : !state.isShowCreateModal,
      };
    },
    setAvailable(state, { payload }) {
      return { ...state, available: payload };
    },
    setSearchString(state, { payload }) {
      return { ...state, searchString: payload };
    },
    setPagination(state, { payload }) {
      return { ...state, pagination: { ...state.pagination, ...payload } };
    },
    resetFilter(state, {}) {
      return {
        ...state,
        pagination: {
          current: 1,
          pageSize: 10,
        },
        searchString: '',
        available: undefined,
      };
    },
  },
  effects: {
    *chunk_list({ payload = {} }, { call, put, select }) {
      const { available, searchString, pagination }: ChunkModelState =
        yield select((state: any) => state.chunkModel);
      const { data } = yield call(kbService.chunk_list, {
        ...payload,
        available_int: available,
        keywords: searchString,
        page: pagination.current,
        size: pagination.pageSize,
      });
      const { retcode, data: res } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            data: res.chunks,
            total: res.total,
            documentInfo: res.doc,
          },
        });
      }
    },
    throttledGetChunkList: [
      function* ({ payload }, { put }) {
        yield put({ type: 'chunk_list', payload: { doc_id: payload } });
      },
      { type: 'throttle', ms: 1000 }, // TODO: Provide type support for this effect
    ],
    *switch_chunk({ payload = {} }, { call }) {
      const { data } = yield call(kbService.switch_chunk, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success('Modified successfully ！');
      }
      return retcode;
    },
    *rm_chunk({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.rm_chunk, payload);
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'setIsShowCreateModal',
          payload: false,
        });
        yield put({ type: 'setPagination', payload: { current: 1 } });
        yield put({ type: 'chunk_list', payload: pick(payload, ['doc_id']) });
      }
      return retcode;
    },
    *get_chunk({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.get_chunk, payload);
      const { retcode, data: res } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            chunkInfo: res,
          },
        });
      }
      return data;
    },
    *create_chunk({ payload = {} }, { call, put }) {
      let service = kbService.create_chunk;
      if (payload.chunk_id) {
        service = kbService.set_chunk;
      }
      const { data } = yield call(service, payload);
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'setIsShowCreateModal',
          payload: false,
        });

        yield put({ type: 'chunk_list', payload: pick(payload, ['doc_id']) });
      }
    },
  },
};
export default model;
