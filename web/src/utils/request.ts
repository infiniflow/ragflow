import { Authorization } from '@/constants/authorization';
import authorizationUtil from '@/utils/authorizationUtil';
import { message, notification } from 'antd';
import { history } from 'umi';
import { RequestMethod, extend } from 'umi-request';

const ABORT_REQUEST_ERR_MESSAGE = 'The user aborted a request.'; // 手动中断请求。errorHandler 抛出的error message

const RetcodeMessage = {
  200: '服务器成功返回请求的数据。',
  201: '新建或修改数据成功。',
  202: '一个请求已经进入后台排队（异步任务）。',
  204: '删除数据成功。',
  400: '发出的请求有错误，服务器没有进行新建或修改数据的操作。',
  401: '用户没有权限（令牌、用户名、密码错误）。',
  403: '用户得到授权，但是访问是被禁止的。',
  404: '发出的请求针对的是不存在的记录，服务器没有进行操作。',
  406: '请求的格式不可得。',
  410: '请求的资源被永久删除，且不会再得到的。',
  422: '当创建一个对象时，发生一个验证错误。',
  500: '服务器发生错误，请检查服务器。',
  502: '网关错误。',
  503: '服务不可用，服务器暂时过载或维护。',
  504: '网关超时。',
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
/**
 * 异常处理程序
 */
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
  // 手动中断请求 abort
  if (error.message === ABORT_REQUEST_ERR_MESSAGE) {
    console.log('user abort  request');
  } else {
    if (response && response.status) {
      const errorText =
        RetcodeMessage[response.status as ResultCode] || response.statusText;
      const { status, url } = response;
      notification.error({
        message: `请求错误 ${status}: ${url}`,
        description: errorText,
      });
    } else if (!response) {
      notification.error({
        description: '您的网络发生异常，无法连接服务器',
        message: '网络异常',
      });
    }
  }
  return response;
};

/**
 * 配置request请求时的默认参数
 */
const request: RequestMethod = extend({
  errorHandler, // 默认错误处理
  timeout: 300000,
  getResponse: true,
});

request.interceptors.request.use((url: string, options: any) => {
  const authorization = authorizationUtil.getAuthorization();
  return {
    url,
    options: {
      ...options,
      headers: {
        ...(options.skipToken ? undefined : { [Authorization]: authorization }),
        ...options.headers,
      },
      interceptors: true,
    },
  };
});

/*
 * 请求response拦截器
 * */

request.interceptors.response.use(async (response: any, options) => {
  if (options.responseType === 'blob') {
    return response;
  }
  const data: ResponseType = await response.clone().json();
  // response 拦截

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
        message: `提示 : ${data.retcode}`,
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
