import { Authorization } from '@/constants/authorization';
import i18n from '@/locales/config';
import authorizationUtil, { getAuthorization } from '@/utils/authorizationUtil';
import { message, notification } from 'antd';
import { history } from 'umi';
import { RequestMethod, extend } from 'umi-request';
import { convertTheKeysOfTheObjectToSnake } from './commonUtil';

const ABORT_REQUEST_ERR_MESSAGE = 'The user aborted a request.';

const RetcodeMessage = {
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
  422: i18n.t('message.422'),
  500: i18n.t('message.500'),
  502: i18n.t('message.502'),
  503: i18n.t('message.503'),
  504: i18n.t('message.504'),
};
type ResultCode =
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
  | 422
  | 500
  | 502
  | 503
  | 504;

interface ResponseType {
  retcode: number;
  data: any;
  retmsg: string;
  status: number;
}
const errorHandler = (error: {
  response: Response;
  message: string;
}): Response => {
  const { response } = error;
  if (error.message === ABORT_REQUEST_ERR_MESSAGE) {
    console.log('user abort  request');
  } else {
    if (response && response.status) {
      const errorText =
        RetcodeMessage[response.status as ResultCode] || response.statusText;
      const { status, url } = response;
      notification.error({
        message: `${i18n.t('message.requestError')} ${status}: ${url}`,
        description: errorText,
      });
    } else if (!response) {
      notification.error({
        description: i18n.t('message.networkAnomalyDescription'),
        message: i18n.t('message.networkAnomaly'),
      });
    }
  }
  return response;
};

const request: RequestMethod = extend({
  errorHandler,
  timeout: 300000,
  getResponse: true,
});

request.interceptors.request.use((url: string, options: any) => {
  const data = convertTheKeysOfTheObjectToSnake(options.data);
  const params = convertTheKeysOfTheObjectToSnake(options.params);

  return {
    url,
    options: {
      ...options,
      data,
      params,
      headers: {
        ...(options.skipToken
          ? undefined
          : { [Authorization]: getAuthorization() }),
        ...options.headers,
      },
      interceptors: true,
    },
  };
});

request.interceptors.response.use(async (response: any, options) => {
  if (options.responseType === 'blob') {
    return response;
  }
  const data: ResponseType = await response.clone().json();

  if (data.retcode === 401 || data.retcode === 401) {
    notification.error({
      message: data.retmsg,
      description: data.retmsg,
      duration: 3,
    });
    authorizationUtil.removeAll();
    history.push('/login'); // Will not jump to the login page
  } else if (data.retcode !== 0) {
    if (data.retcode === 100) {
      message.error(data.retmsg);
    } else {
      notification.error({
        message: `${i18n.t('message.hint')} : ${data.retcode}`,
        description: data.retmsg,
        duration: 3,
      });
    }

    return response;
  } else {
    return response;
  }
});

export default request;
