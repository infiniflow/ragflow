import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import kbService from '@/services/kbService';
import { message } from 'antd';
// import { delay } from '@/utils/storeUtil';
import { DvaModel } from 'umi';

export interface ChunkModelState {
  data: any[];
  total: number;
  isShowCreateModal: boolean;
  chunk_id: string;
  doc_id: string;
  chunkInfo: any;
  documentInfo: Partial<IKnowledgeFile>;
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
  //     history.listen(location => {
  //       console.log(location)
  //     });
  //   }
  // },
  effects: {
    *chunk_list({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.chunk_list, payload);
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
