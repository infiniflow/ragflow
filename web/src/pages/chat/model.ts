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
  effects: {
    *query({ payload }, { call, put }) {},
  },
  subscriptions: {
    setup({ dispatch, history }) {
      return history.listen((query) => {
        console.log(query);
      });
    },
  },
};

export default model;
