import { DvaModel } from 'umi';

export interface ChatModelState {
  name: string;
}

const model: DvaModel<ChatModelState> = {
  namespace: 'chatModel',
  state: {
    name: 'kate',
  },
  reducers: {
    save(state, action) {
      return {
        ...state,
        ...action.payload,
      };
    },
  },
  subscriptions: {
    setup({ dispatch, history }) {
      return history.listen((query) => {
        console.log(query);
      });
    },
  },
  effects: {
    *query({ payload }, { call, put }) {},
  },
};

export default model;
