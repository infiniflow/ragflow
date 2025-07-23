import message from '@/components/ui/message';
import { Authorization } from '@/constants/authorization';
import i18n from '@/locales/config';
import authorizationUtil, {
  getAuthorization,
  redirectToLogin,
} from '@/utils/authorization-util';
import { notification } from 'antd';
import axios from 'axios';
import { convertTheKeysOfTheObjectToSnake } from './common-util';

const FAILED_TO_FETCH = 'Failed to fetch';

export const RetcodeMessage = {
  200: i18n.t('message.200'),
  201: i18n.t('message.201'),
  202: i18n.t('message.202'),
  204: i18n.t('message.204'),
  400: i18n.t('message.400'),
  401: i18n.t('message.401'),
  403: i18n.t('message.403'),
  404: i18n.t('message.404'),
  406: i18n.t('message.406'),
  410: i18n.t('message.410'),
  413: i18n.t('message.413'),
  422: i18n.t('message.422'),
  500: i18n.t('message.500'),
  502: i18n.t('message.502'),
  503: i18n.t('message.503'),
  504: i18n.t('message.504'),
};
export type ResultCode =
  | 200
  | 201
  | 202
  | 204
  | 400
  | 401
  | 403
  | 404
  | 406
  | 410
  | 413
  | 422
  | 500
  | 502
  | 503
  | 504;

const errorHandler = (error: {
  response: Response;
  message: string;
}): Response => {
  const { response } = error;
  if (error.message === FAILED_TO_FETCH) {
    notification.error({
      description: i18n.t('message.networkAnomalyDescription'),
      message: i18n.t('message.networkAnomaly'),
    });
  } else {
    if (response && response.status) {
      const errorText =
        RetcodeMessage[response.status as ResultCode] || response.statusText;
      const { status, url } = response;
      notification.error({
        message: `${i18n.t('message.requestError')} ${status}: ${url}`,
        description: errorText,
      });
    }
  }
  return response ?? { data: { code: 1999 } };
};

const request = axios.create({
  //   errorHandler,
  timeout: 300000,
  //   getResponse: true,
});

request.interceptors.request.use(
  (config) => {
    const data = convertTheKeysOfTheObjectToSnake(config.data);
    const params = convertTheKeysOfTheObjectToSnake(config.params);

    const newConfig = { ...config, data, params };

    if (!newConfig.skipToken) {
      newConfig.headers.set(Authorization, getAuthorization());
    }

    return newConfig;
  },
  function (error) {
    return Promise.reject(error);
  },
);

request.interceptors.response.use(
  async (response) => {
    if (response?.status === 413 || response?.status === 504) {
      message.error(RetcodeMessage[response?.status as ResultCode]);
    }

    if (response.config.responseType === 'blob') {
      return response;
    }
    const data = response?.data;
    if (data?.code === 100) {
      message.error(data?.message);
    } else if (data?.code === 401) {
      notification.error({
        message: data?.message,
        description: data?.message,
        duration: 3,
      });
      authorizationUtil.removeAll();
      redirectToLogin();
    } else if (data?.code !== 0) {
      notification.error({
        message: `${i18n.t('message.hint')} : ${data?.code}`,
        description: data?.message,
        duration: 3,
      });
    }
    return response;
  },
  function (error) {
    console.log('🚀 ~ error:', error);
    errorHandler(error);
    return Promise.reject(error);
  },
);

export default request;

export const get = (url: string) => {
  return request.get(url);
};

export const post = (url: string, body: any) => {
  return request.post(url, { data: body });
};

export const drop = () => {};

export const put = () => {};
