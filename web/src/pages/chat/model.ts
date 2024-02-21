import { IDialog } from '@/interfaces/database/chat';
import chatService from '@/services/chatService';
import { message } from 'antd';
import { DvaModel } from 'umi';

export interface ChatModelState {
  name: string;
  dialogList: IDialog[];
  currentDialog: IDialog;
}

const model: DvaModel<ChatModelState> = {
  namespace: 'chatModel',
  state: {
    name: 'kate',
    dialogList: [],
    currentDialog: <IDialog>{},
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
    setCurrentDialog(state, { payload }) {
      return {
        ...state,
        currentDialog: payload,
      };
    },
  },

  effects: {
    *getDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.getDialog, payload);
      if (data.retcode === 0) {
        yield put({ type: 'setCurrentDialog', payload: data.data });
      }
    },
    *setDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.setDialog, payload);
      if (data.retcode === 0) {
        yield put({ type: 'listDialog' });
        message.success('Created successfully !');
      }
      return data.retcode;
    },
    *removeDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.removeDialog, payload);
      if (data.retcode === 0) {
        yield put({ type: 'listDialog' });
        message.success('Deleted successfully !');
      }
      return data.retcode;
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
