import { IConversation, IDialog, Message } from '@/interfaces/database/chat';
import i18n from '@/locales/config';
import chatService from '@/services/chat-service';
import { message } from 'antd';
import { DvaModel } from 'umi';
import { v4 as uuid } from 'uuid';
import { IClientConversation, IMessage } from './interface';
import { getDocumentIdsFromConversionReference } from './utils';

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
  },

  effects: {
    *getDialog({ payload }, { call, put }) {
      const needToBeSaved =
        payload.needToBeSaved === undefined ? true : payload.needToBeSaved;
      const { data } = yield call(chatService.getDialog, {
        dialog_id: payload.dialog_id,
      });
      if (data.retcode === 0 && needToBeSaved) {
        yield put({ type: 'setCurrentDialog', payload: data.data });
      }
      return data;
    },
    *setDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.setDialog, payload);
      if (data.retcode === 0) {
        yield put({ type: 'listDialog' });
        message.success(
          i18n.t(`message.${payload.dialog_id ? 'modified' : 'created'}`),
        );
      }
      return data.retcode;
    },
    *removeDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.removeDialog, payload);
      if (data.retcode === 0) {
        yield put({ type: 'listDialog' });
        message.success(i18n.t('message.deleted'));
      }
      return data.retcode;
    },
    *listDialog({ payload }, { call, put }) {
      const { data } = yield call(chatService.listDialog, payload);
      if (data.retcode === 0) {
        yield put({ type: 'setDialogList', payload: data.data });
      }
      return data;
    },
    *listConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.listConversation, payload);
      if (data.retcode === 0) {
        yield put({ type: 'setConversationList', payload: data.data });
      }
      return data.retcode;
    },
    *getConversation({ payload }, { call, put }) {
      const needToBeSaved =
        payload.needToBeSaved === undefined ? true : payload.needToBeSaved;
      const { data } = yield call(chatService.getConversation, {
        conversation_id: payload.conversation_id,
      });
      if (data.retcode === 0 && needToBeSaved) {
        yield put({
          type: 'kFModel/fetch_document_thumbnails',
          payload: {
            doc_ids: getDocumentIdsFromConversionReference(data.data),
          },
        });
        yield put({ type: 'setCurrentConversation', payload: data.data });
      }
      return data;
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
    *completeConversation({ payload }, { call }) {
      const { data } = yield call(chatService.completeConversation, payload);
      // if (data.retcode === 0) {
      //   yield put({
      //     type: 'getConversation',
      //     payload: {
      //       conversation_id: payload.conversation_id,
      //     },
      //   });
      // }
      return data.retcode;
    },
    *removeConversation({ payload }, { call, put }) {
      const { data } = yield call(chatService.removeConversation, {
        conversation_ids: payload.conversation_ids,
      });
      if (data.retcode === 0) {
        yield put({
          type: 'listConversation',
          payload: { dialog_id: payload.dialog_id },
        });
        message.success(i18n.t('message.deleted'));
      }
      return data.retcode;
    },
    *createExternalConversation({ payload }, { call, put }) {
      const { data } = yield call(
        chatService.createExternalConversation,
        payload,
      );
      // if (data.retcode === 0) {
      //   yield put({
      //     type: 'getExternalConversation',
      //     payload: data.data.id,
      //   });
      // }
      return data;
    },
    *getExternalConversation({ payload }, { call }) {
      const { data } = yield call(
        chatService.getExternalConversation,
        null,
        payload,
      );
      return data;
    },
    *completeExternalConversation({ payload }, { call }) {
      const { data } = yield call(
        chatService.completeExternalConversation,
        payload,
      );
      return data.retcode;
    },
  },
};

export default model;
