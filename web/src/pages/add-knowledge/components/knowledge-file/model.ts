import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import kbService from '@/services/kbService';
import { message } from 'antd';
import omit from 'lodash/omit';
import pick from 'lodash/pick';
import { Nullable } from 'typings';
import { DvaModel } from 'umi';

export interface KFModelState {
  isShowCEFwModal: boolean;
  isShowTntModal: boolean;
  isShowSegmentSetModal: boolean;
  isShowRenameModal: boolean;
  tenantIfo: any;
  data: IKnowledgeFile[];
  currentRecord: Nullable<IKnowledgeFile>;
}

const model: DvaModel<KFModelState> = {
  namespace: 'kFModel',
  state: {
    isShowCEFwModal: false,
    isShowTntModal: false,
    isShowSegmentSetModal: false,
    isShowRenameModal: false,
    tenantIfo: {},
    data: [],
    currentRecord: null,
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
    setIsShowRenameModal(state, { payload }) {
      return { ...state, isShowRenameModal: payload };
    },
    setCurrentRecord(state, { payload }) {
      return { ...state, currentRecord: payload };
    },
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen((location) => {});
    },
  },
  effects: {
    *createKf({ payload = {} }, { call, put }) {
      const { data, response } = yield call(kbService.createKb, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('创建成功！');
      }
    },
    *updateKf({ payload = {} }, { call, put }) {
      const { data, response } = yield call(kbService.updateKb, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('修改成功！');
      }
    },
    *getKfDetail({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.get_kb_detail, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        // localStorage.setItem('userInfo',res.)
        callback && callback(res);
      }
    },
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
            data: res,
          },
        });
      }
    },
    *updateDocumentStatus({ payload = {} }, { call, put }) {
      const { data, response } = yield call(
        kbService.document_change_status,
        pick(payload, ['doc_id', 'status']),
      );
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('修改成功！');
        put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }
    },
    *document_rm({ payload = {} }, { call, put }) {
      const { data, response } = yield call(kbService.document_rm, {
        doc_id: payload.doc_id,
      });
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('删除成功！');
        put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }
    },
    *document_rename({ payload = {} }, { call, put }) {
      const { data } = yield call(
        kbService.document_rename,
        omit(payload, ['kb_id']),
      );
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        message.success('rename success！');
        yield put({
          type: 'setIsShowRenameModal',
          payload: false,
        });
        yield put({
          type: 'getKfList',
          payload: { kb_id: payload.kb_id },
        });
      }

      return retcode;
    },
    *document_create({ payload = {} }, { call, put }) {
      const { data, response } = yield call(kbService.document_create, payload);
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        put({
          type: 'kFModel/updateState',
          payload: {
            isShowCEFwModal: false,
          },
        });
        message.success('创建成功！');
      }
      return retcode;
    },
    *document_change_parser({ payload = {} }, { call, put }) {
      const { data, response } = yield call(
        kbService.document_change_parser,
        payload,
      );
      const { retcode, data: res, retmsg } = data;
      if (retcode === 0) {
        put({
          type: 'updateState',
          payload: {
            isShowSegmentSetModal: false,
          },
        });
        message.success('修改成功！');
      }
      return retcode;
    },
  },
};
export default model;
