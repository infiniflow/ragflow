import { message } from 'antd';
import { addParam } from '@/utils';
import kbService from '@/services/kbService';

const Model = {
  namespace: 'kFModel',
  state: {
    isShowCEFwModal: false,
    isShowTntModal: false,
    loading: false,
    tenantIfo: {}
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


      }
    },
    * updateKf({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.updateKb, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {


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
      yield put({
        type: 'updateState',
        payload: {
          loading: false
        }
      });
      callback && callback()
    },
    *document_rm({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.document_rm, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        callback && callback()
      }

    },
    *document_create({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(kbService.document_create, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
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
