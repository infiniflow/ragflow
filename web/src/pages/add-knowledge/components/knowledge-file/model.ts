import { BaseState } from '@/interfaces/common';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import i18n from '@/locales/config';
import kbService, { getDocumentFile } from '@/services/knowledge-service';
import { message } from 'antd';
import omit from 'lodash/omit';
import pick from 'lodash/pick';
import { DvaModel } from 'umi';

export interface KFModelState extends BaseState {
  tenantIfo: any;
  data: IKnowledgeFile[];
  total: number;
  currentRecord: Nullable<IKnowledgeFile>;
  fileThumbnails: Record<string, string>;
}

const model: DvaModel<KFModelState> = {
  namespace: 'kFModel',
  state: {
    tenantIfo: {},
    data: [],
    total: 0,
    currentRecord: null,
    searchString: '',
    pagination: {
      current: 1,
      pageSize: 10,
    },
    fileThumbnails: {} as Record<string, string>,
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },

    setCurrentRecord(state, { payload }) {
      return { ...state, currentRecord: payload };
    },
    setSearchString(state, { payload }) {
      return { ...state, searchString: payload };
    },
    setPagination(state, { payload }) {
      return { ...state, pagination: { ...state.pagination, ...payload } };
    },
    setFileThumbnails(state, { payload }) {
      return { ...state, fileThumbnails: payload };
    },
  },
  effects: {
    *createKf({ payload = {} }, { call }) {
      const { data } = yield call(kbService.createKb, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.created'));
      }
    },
    *updateKf({ payload = {} }, { call }) {
      const { data } = yield call(kbService.updateKb, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.modified'));
      }
    },
    *getKfDetail({ payload = {} }, { call }) {
      const { data } = yield call(kbService.get_kb_detail, payload);
    },
    *getKfList({ payload = {} }, { call, put, select }) {
      const state: KFModelState = yield select((state: any) => state.kFModel);
      const requestBody = {
        ...payload,
        page: state.pagination.current,
        page_size: state.pagination.pageSize,
      };
      if (state.searchString) {
        requestBody['keywords'] = state.searchString;
      }
      const { data } = yield call(kbService.get_document_list, requestBody);
      const { retcode, data: res } = data;

      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            data: res.docs,
            total: res.total,
          },
        });
      }
    },
    throttledGetDocumentList: [
      function* ({ payload }, { call, put }) {
        yield put({ type: 'getKfList', payload: { kb_id: payload } });
      },
      { type: 'throttle', ms: 1000 }, // TODO: Provide type support for this effect
    ],
    pollGetDocumentList: [
      function* ({ payload }, { call, put }) {
        yield put({ type: 'getKfList', payload: { kb_id: payload } });
      },
      { type: 'poll', delay: 15000 }, // TODO: Provide type support for this effect
    ],
    *updateDocumentStatus({ payload = {} }, { call, put }) {
      const { data } = yield call(
        kbService.document_change_status,
        pick(payload, ['doc_id', 'status']),
      );
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.modified'));
        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }
    },
    *document_rm({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.document_rm, {
        doc_id: payload.doc_id,
      });
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.deleted'));
        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }
      return retcode;
    },
    *document_rename({ payload = {} }, { call, put }) {
      const { data } = yield call(
        kbService.document_rename,
        omit(payload, ['kb_id']),
      );
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.renamed'));

        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }

      return retcode;
    },
    *document_create({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.document_create, payload);
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });

        message.success(i18n.t('message.created'));
      }
      return retcode;
    },
    *document_run({ payload = {} }, { call, put }) {
      const { data } = yield call(
        kbService.document_run,
        omit(payload, ['knowledgeBaseId']),
      );
      const { retcode } = data;
      if (retcode === 0) {
        if (payload.knowledgeBaseId) {
          yield put({
            type: 'getKfList',
            payload: { kb_id: payload.knowledgeBaseId },
          });
        }
        message.success(i18n.t('message.operated'));
      }
      return retcode;
    },
    *document_change_parser({ payload = {} }, { call, put }) {
      const { data } = yield call(
        kbService.document_change_parser,
        omit(payload, ['kb_id']),
      );
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });

        message.success(i18n.t('message.modified'));
      }
      return retcode;
    },
    *fetch_document_thumbnails({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.document_thumbnails, payload);
      if (data.retcode === 0) {
        yield put({ type: 'setFileThumbnails', payload: data.data });
      }
    },
    *fetch_document_file({ payload = {} }, { call }) {
      const documentId = payload;
      try {
        const ret = yield call(getDocumentFile, documentId);
        return ret;
      } catch (error) {
        console.warn(error);
      }
    },
    *upload_document({ payload = {} }, { call, put }) {
      const fileList = payload.fileList;
      const formData = new FormData();
      formData.append('kb_id', payload.kb_id);
      fileList.forEach((file: any) => {
        formData.append('file', file);
      });

      const ret = yield call(kbService.document_upload, formData);

      const succeed = ret?.data?.retcode === 0;

      if (succeed) {
        message.success(i18n.t('message.uploaded'));
      }
      if (succeed || ret?.data?.retcode === 500) {
        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }
      return ret?.data;
    },
    *web_crawl({ payload = {} }, { call, put }) {
      const formData = new FormData();
      formData.append('name', payload.name);
      formData.append('url', payload.url);
      formData.append('kb_id', payload.kb_id);

      const { data } = yield call(kbService.web_crawl, formData);

      const succeed = data.retcode === 0;

      if (succeed) {
        message.success(i18n.t('message.uploaded'));
      }
      if (succeed || data.retcode === 500) {
        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }
      return data.retcode;
    },
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(({ location }) => {
        const state: { from: string } = (location.state ?? {
          from: '',
        }) as { from: string };
        if (
          state.from === '/knowledge' || // TODO: Just directly determine whether the current page is on the knowledge list page.
          location.pathname === '/knowledge/dataset/upload'
        ) {
          dispatch({
            type: 'setPagination',
            payload: { current: 1, pageSize: 10 },
          });
        }
      });
    },
  },
};
export default model;
