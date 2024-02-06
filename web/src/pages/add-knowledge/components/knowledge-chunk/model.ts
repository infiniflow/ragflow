import { BaseState } from '@/interfaces/common';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import kbService from '@/services/kbService';
import { message } from 'antd';
// import { delay } from '@/utils/storeUtil';
import { DvaModel } from 'umi';

export interface ChunkModelState extends BaseState {
  data: any[];
  total: number;
  isShowCreateModal: boolean;
  chunk_id: string;
  doc_id: string;
  chunkInfo: any;
  documentInfo: Partial<IKnowledgeFile>;
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
    documentInfo: {},
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
    setAvailable(state, { payload }) {
      return { ...state, available: payload };
    },
    setSearchString(state, { payload }) {
      return { ...state, searchString: payload };
    },
    setPagination(state, { payload }) {
      return { ...state, pagination: { ...state.pagination, ...payload } };
    },
    resetFilter(state, { payload }) {
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
    *switch_chunk({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.switch_chunk, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success('Modified successfully ！');
      }
      return retcode;
    },
    *rm_chunk({ payload = {} }, { call, put }) {
      console.log('shanchu');
      const { data, response } = yield call(kbService.rm_chunk, payload);
      const { retcode, data: res, retmsg } = data;

      return retcode;
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
      return data;
    },
    *create_hunk({ payload = {} }, { call, put }) {
      let service = kbService.create_chunk;
      if (payload.chunk_id) {
        service = kbService.set_chunk;
      }
      const { data, response } = yield call(service, payload);
      const { retcode, data: res, retmsg } = data;
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
