import { message } from 'antd';
import { addParam } from '@/utils';
import kbService from '@/services/kbService';

const Model = {
  namespace: 'kAModel',
  state: {
    isShowPSwModal: false,
    isShowTntModal: false,
    loading: false,
    tenantIfo: {},
    activeKey: 'setting',
    id: ''

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
