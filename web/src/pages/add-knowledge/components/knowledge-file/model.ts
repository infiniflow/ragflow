import { message } from 'antd';
import { Effect, Reducer, Subscription } from 'umi'
import kbService from '@/services/kbService';

export interface kFModelState {
  isShowCEFwModal: boolean;
  isShowTntModal: boolean;
  isShowSegmentSetModal: boolean;
  loading: boolean;
  tenantIfo: any;
  data: any[]
}
export interface kFModelType {
  namespace: 'kFModel';
  state: kFModelState;
  effects: {
    createKf: Effect;
    updateKf: Effect;
    getKfDetail: Effect;
    getKfList: Effect;
    updateDocumentStatus: Effect;
    document_rm: Effect;
    document_create: Effect;
    document_change_parser: Effect;
  };
  reducers: {
    updateState: Reducer<kFModelState>;
  };
  subscriptions: { setup: Subscription };
}
const Model: kFModelType = {
  namespace: 'kFModel',
  state: {
    isShowCEFwModal: false,
    isShowTntModal: false,
    isShowSegmentSetModal: false,
    loading: false,
    tenantIfo: {},
    data: []
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(location => {
      });
    }
  },
  effects: {
    * createKf({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.createKb, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {

        message.success('创建成功！');
      }
    },
    * updateKf({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.updateKb, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('修改成功！');

      }
    },
    *getKfDetail({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.get_kb_detail, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        // localStorage.setItem('userInfo',res.)
        callback && callback(res)
      }
    },
    *getKfList({ payload = {} }, { call, put }) {
      yield put({
        type: 'updateState',
        payload: {
          loading: true
        }
      });
      const { data, response } = yield call(kbService.get_document_list, payload);
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
            data: res
          }
        });
      }
    },
    *updateDocumentStatus({ payload = {}, callback }, { call, put }) {
      yield put({
        type: 'updateState',
        payload: {
          loading: true
        }
      });
      const { data, response } = yield call(kbService.document_change_status, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('修改成功！');
        yield put({
          type: 'updateState',
          payload: {
            loading: false
          }
        });
        callback && callback()
      }

    },
    *document_rm({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.document_rm, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('删除成功！');
        callback && callback()
      }

    },
    *document_create({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.document_create, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('创建成功！');
        callback && callback()
      }

    },
    *document_change_parser({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.document_change_parser, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('修改成功！');
        callback && callback()
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
