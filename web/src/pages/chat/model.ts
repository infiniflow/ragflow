import { IDialog } from '@/interfaces/database/chat';
import chatService from '@/services/chatService';
import { message } from 'antd';
import { DvaModel } from 'umi';

export interface ChatModelState {
  name: string;
  dialogList: IDialog[];
}

const model: DvaModel<ChatModelState> = {
  namespace: 'chatModel',
  state: {
    name: 'kate',
    dialogList: [],
  },
  reducers: {
    save(state, action) {
      return {
        ...state,
        ...action.payload,
      };
    },
    setDialogList(state, { payload }) {
      return {
        ...state,
        dialogList: payload,
      };
    },
  },

  effects: {
    *getDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.getDialog, payload);
    },
    *setDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.setDialog, payload);
      if (data.retcode === 0) {
        yield put({ type: 'listDialog' });
        message.success('Created successfully !');
      }
    },
    *listDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.listDialog, payload);
      yield put({ type: 'setDialogList', payload: data.data });
    },
    *listConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.listConversation, payload);
    },
    *getConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.getConversation, payload);
    },
    *setConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.setConversation, payload);
    },
    *completeConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.completeConversation, payload);
    },
  },
};

export default model;
