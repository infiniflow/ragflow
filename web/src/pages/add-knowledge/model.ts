import { Effect, Reducer, Subscription } from 'umi'
import { message } from 'antd';
import kbService from '@/services/kbService';
export interface kAModelState {
  isShowPSwModal: boolean;
  isShowTntModal: boolean;
  loading: boolean;
  tenantIfo: any;
  activeKey: string;
  id: string;
  doc_id: string
}
export interface kAModelType {
  namespace: 'kAModel';
  state: kAModelState;
  effects: {

  };
  reducers: {
    updateState: Reducer<kAModelState>;
  };
  subscriptions: { setup: Subscription };
}
const Model: kAModelType = {
  namespace: 'kAModel',
  state: {
    isShowPSwModal: false,
    isShowTntModal: false,
    loading: false,
    tenantIfo: {},
    activeKey: 'setting',
    id: '',
    doc_id: ''

  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(location => {
      });
    }
  },
  effects: {

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
