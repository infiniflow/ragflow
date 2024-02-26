import { IConversation, IDialog, Message } from '@/interfaces/database/chat';
import chatService from '@/services/chatService';
import { message } from 'antd';
import { DvaModel } from 'umi';
import { v4 as uuid } from 'uuid';
import { IClientConversation, IMessage } from './interface';

export interface ChatModelState {
  name: string;
  dialogList: IDialog[];
  currentDialog: IDialog;
  conversationList: IConversation[];
  currentConversation: IClientConversation;
}

const model: DvaModel<ChatModelState> = {
  namespace: 'chatModel',
  state: {
    name: 'kate',
    dialogList: [],
    currentDialog: <IDialog>{},
    conversationList: [],
    currentConversation: {} as IClientConversation,
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
    setConversationList(state, { payload }) {
      return {
        ...state,
        conversationList: payload,
      };
    },
    setCurrentConversation(state, { payload }) {
      const messageList =
        payload?.message?.map((x: Message | IMessage) => ({
          ...x,
          id: 'id' in x ? x.id : uuid(),
        })) ?? [];
      return {
        ...state,
        currentConversation: { ...payload, message: messageList },
      };
    },
    addEmptyConversationToList(state, {}) {
      const list = [...state.conversationList];
      // if (list.every((x) => x.id !== 'empty')) {
      //   list.push({
      //     id: 'empty',
      //     name: 'New conversation',
      //     message: [],
      //   });
      // }
      return {
        ...state,
        conversationList: list,
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
      if (data.retcode === 0) {
        yield put({ type: 'setConversationList', payload: data.data });
      }
      return data.retcode;
    },
    *getConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.getConversation, payload);
      if (data.retcode === 0) {
        yield put({ type: 'setCurrentConversation', payload: data.data });
      }
      return data.retcode;
    },
    *setConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.setConversation, payload);
      if (data.retcode === 0) {
        yield put({
          type: 'listConversation',
          payload: {
            dialog_id: data.data.dialog_id,
          },
        });
      }
      return data;
    },
    *completeConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.completeConversation, payload);
      if (data.retcode === 0) {
        yield put({
          type: 'getConversation',
          payload: {
            conversation_id: payload.conversation_id,
          },
        });
      }
    },
  },
};

export default model;
