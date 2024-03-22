import { ITenantInfo } from '@/interfaces/database/knowledge';
import {
  IFactory,
  IMyLlmValue,
  IThirdOAIModelCollection as IThirdAiModelCollection,
} from '@/interfaces/database/llm';
import { IUserInfo } from '@/interfaces/database/userSetting';
import userService from '@/services/userService';
import { message } from 'antd';
import { DvaModel } from 'umi';

export interface SettingModelState {
  llm_factory: string;
  tenantIfo: Nullable<ITenantInfo>;
  llmInfo: IThirdAiModelCollection;
  myLlmList: Record<string, IMyLlmValue>;
  factoryList: IFactory[];
  userInfo: IUserInfo;
}

const model: DvaModel<SettingModelState> = {
  namespace: 'settingModel',
  state: {
    llm_factory: '',
    tenantIfo: null,
    llmInfo: {},
    myLlmList: {},
    factoryList: [],
    userInfo: {} as IUserInfo,
  },
  reducers: {
    updateState(state, { payload }) {
      return {
        ...state,
        ...payload,
      };
    },
    setUserInfo(state, { payload }) {
      return {
        ...state,
        userInfo: payload,
      };
    },
  },
  effects: {
    *setting({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.setting, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success('Modified!');
        yield put({
          type: 'getUserInfo',
        });
      }
    },
    *getUserInfo({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.user_info, payload);
      const { retcode, data: res } = data;

      // const userInfo = {
      //   avatar: res.avatar,
      //   name: res.nickname,
      //   email: res.email,
      // };
      // authorizationUtil.setUserInfo(userInfo);
      if (retcode === 0) {
        yield put({ type: 'setUserInfo', payload: res });
        // localStorage.setItem('userInfo',res.)
      }
    },
    *getTenantInfo({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.get_tenant_info, payload);
      const { retcode, data: res } = data;
      // llm_id 对应chat_id
      // asr_id 对应speech2txt

      if (retcode === 0) {
        res.chat_id = res.llm_id;
        res.speech2text_id = res.asr_id;
        yield put({
          type: 'updateState',
          payload: {
            tenantIfo: res,
          },
        });
      }
    },
    *set_tenant_info({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.set_tenant_info, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success('Modified!');
        yield put({
          type: 'getTenantInfo',
        });
      }
      return retcode;
    },

    *factories_list({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.factories_list);
      const { retcode, data: res } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            factoryList: res,
          },
        });
      }
    },
    *llm_list({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.llm_list, payload);
      const { retcode, data: res } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            llmInfo: res,
          },
        });
      }
    },
    *my_llm({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.my_llm);
      const { retcode, data: res } = data;
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            myLlmList: res,
          },
        });
      }
    },
    *set_api_key({ payload = {} }, { call, put }) {
      const { data } = yield call(userService.set_api_key, payload);
      const { retcode } = data;
      if (retcode === 0) {
        message.success('Modified!');
        yield put({ type: 'my_llm' });
        yield put({ type: 'factories_list' });
        yield put({
          type: 'updateState',
        });
      }
      return retcode;
    },
  },
};
export default model;
