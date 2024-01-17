import { Effect, Reducer, Subscription } from 'umi';
import { message } from 'antd';
import { addParam } from '@/utils';
import userService from '@/services/userService';
import { rearg } from 'lodash';

export interface settingModelState {
  isShowPSwModal: boolean;
  isShowTntModal: boolean;
  isShowSAKModal: boolean;
  isShowSSModal: boolean;
  llm_factory: string;
  loading: boolean;
  tenantIfo: any,
  llmInfo: any,
  myLlm: any[],
  factoriesList: any[]
}

export interface settingModelType {
  namespace: 'settingModel';
  state: settingModelState;
  effects: {
    setting: Effect;
    getUserInfo: Effect;
    getTenantInfo: Effect;
    set_tenant_info: Effect;
    factories_list: Effect;
    llm_list: Effect;
    my_llm: Effect;
    set_api_key: Effect;
  };
  reducers: {
    updateState: Reducer<settingModelState>;
  };
  subscriptions: { setup: Subscription };
}
const Model: settingModelType = {
  namespace: 'settingModel',
  state: {
    isShowPSwModal: false,
    isShowTntModal: false,
    isShowSAKModal: false,
    isShowSSModal: false,
    llm_factory: '',
    loading: false,
    tenantIfo: {},
    llmInfo: {},
    myLlm: [],
    factoriesList: []
  },
  subscriptions: {
    setup({ dispatch, history }) {
      history.listen(location => {
      });
    }
  },
  effects: {
    *setting({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(userService.setting, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('密码修改成功！');
        callback && callback()
      }
    },
    *getUserInfo({ payload = {} }, { call, put }) {
      const { data, response } = yield call(userService.user_info, payload);
      const { retcode, data: res, retmsg } = data
      const userInfo = {
        avatar: res.avatar,
        name: res.nickname,
        email: res.email
      };
      localStorage.setItem('userInfo', JSON.stringify(userInfo))
      if (retcode === 0) {
        // localStorage.setItem('userInfo',res.)
      }
    },
    *getTenantInfo({ payload = {} }, { call, put }) {
      yield put({
        type: 'updateState',
        payload: {
          loading: true
        }
      });
      const { data, response } = yield call(userService.get_tenant_info, payload);
      const { retcode, data: res, retmsg } = data
      // llm_id 对应chat_id
      // asr_id 对应speech2txt

      yield put({
        type: 'updateState',
        payload: {
          loading: false
        }
      });
      if (retcode === 0) {
        res.chat_id = res.llm_id
        res.speech2text_id = res.asr_id
        yield put({
          type: 'updateState',
          payload: {
            tenantIfo: res
          }
        });
      }
    },
    *set_tenant_info({ payload = {} }, { call, put }) {
      const { data, response } = yield call(userService.set_tenant_info, payload);
      const { retcode, data: res, retmsg } = data
      // llm_id 对应chat_id
      // asr_id 对应speech2txt
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            isShowSSModal: false
          }
        });
        yield put({
          type: 'getTenantInfo'
        })
      }
    },

    *factories_list({ payload = {} }, { call, put }) {
      const { data, response } = yield call(userService.factories_list, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            factoriesList: res
          }
        });
      }
    },
    *llm_list({ payload = {} }, { call, put }) {
      const { data, response } = yield call(userService.llm_list, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            llmInfo: res
          }
        });
      }
    },
    *my_llm({ payload = {} }, { call, put }) {
      const { data, response } = yield call(userService.my_llm, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        yield put({
          type: 'updateState',
          payload: {
            myLlm: res
          }
        });
      }
    },
    *set_api_key({ payload = {}, callback }, { call, put }) {
      const { data, response } = yield call(userService.set_api_key, payload);
      const { retcode, data: res, retmsg } = data
      if (retcode === 0) {
        message.success('设置API KEY成功！');
        callback && callback()
      }
    },
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
