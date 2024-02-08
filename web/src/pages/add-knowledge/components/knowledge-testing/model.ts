import { BaseState } from '@/interfaces/common';
import {
  ITestingChunk,
  ITestingDocument,
} from '@/interfaces/database/knowledge';
import kbService from '@/services/kbService';
import { DvaModel } from 'umi';

export interface TestingModelState extends Pick<BaseState, 'pagination'> {
  chunks: ITestingChunk[];
  documents: ITestingDocument[];
  total: number;
  selectedDocumentIds: string[] | undefined;
}

const initialState = {
  chunks: [],
  documents: [],
  total: 0,
  pagination: {
    current: 1,
    pageSize: 10,
  },
  selectedDocumentIds: undefined,
};

const model: DvaModel<TestingModelState> = {
  namespace: 'testingModel',
  state: initialState,
  reducers: {
    setChunksAndDocuments(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
    setPagination(state, { payload }) {
      return { ...state, pagination: { ...state.pagination, ...payload } };
    },
    setSelectedDocumentIds(state, { payload }) {
      return { ...state, selectedDocumentIds: payload };
    },
    reset() {
      return initialState;
    },
  },
  effects: {
    *testDocumentChunk({ payload = {} }, { call, put, select }) {
      const { pagination, selectedDocumentIds }: TestingModelState =
        yield select((state: any) => state.testingModel);

      const { data } = yield call(kbService.retrieval_test, {
        ...payload,
        doc_ids: selectedDocumentIds,
        page: pagination.current,
        size: pagination.pageSize,
      });
      const { retcode, data: res } = data;
      if (retcode === 0) {
        yield put({
          type: 'setChunksAndDocuments',
          payload: {
            chunks: res.chunks,
            documents: res.doc_aggs,
            total: res.total,
          },
        });
      }
    },
  },
};
export default model;
