import { DvaModel } from 'umi';
export interface kAModelState {
  isShowPSwModal: boolean;
  tenantIfo: any;
  id: string;
  doc_id: string;
}

const model: DvaModel<kAModelState> = {
  namespace: 'kAModel',
  state: {
    isShowPSwModal: false,
    tenantIfo: {},
    id: '',
    doc_id: '',
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen((location) => {});
    },
  },
  effects: {},
};
export default model;
