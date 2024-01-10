/**
 * request 网络请求工具
 * 更详细的 api 文档: https://github.com/umijs/umi-request
 */
import { extend } from 'umi-request';
import { notification, message } from 'antd';
import store from '@/utils/persistStore';
import config from '@/utils/config';
import _ from 'lodash';

import api from '@/utils/api';
const { login } = api;

const ABORT_REQUEST_ERR_MESSAGE = 'The user aborted a request.'; // 手动中断请求。errorHandler 抛出的error message



const codeMessage = {
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
  504: '网关超时。'
};

/**
 * 异常处理程序
 */
const errorHandler = (error: any) => {
  const { response } = error;
  // 手动中断请求 abort
  if (error.message === ABORT_REQUEST_ERR_MESSAGE) {
    console.log('user abort  request');
  } else {
    if (response && response.status) {
      const errorText = codeMessage[response.status] || response.statusText;
      const { status, url } = response;
      notification.error({
        message: `请求错误 ${status}: ${url}`,
        description: errorText,
        top: 65
      });
    } else if (!response) {
      notification.error({
        description: '您的网络发生异常，无法连接服务器',
        message: '网络异常',
        top: 65
      });
    }
  }
  return response;
};

/**
 * 配置request请求时的默认参数
 */
const request = extend({
  errorHandler, // 默认错误处理
  // credentials: 'include', // 默认请求是否带上cookie
  timeout: 3000000
});

request.interceptors.request.use((url, options) => {
  let prefix = '';
  console.log(url)
  return {
    url,
    options: {
      ...options,
      headers: {
        ...(options.skipToken ? undefined : { Authorization: 'Bearer ' + store.token }),
        ...options.headers
      },
      interceptors: true
    }
  };
});

/*
 * 请求response拦截器
 * */
request.interceptors.response.use(async (response, request) => {
  const data = await response.clone().json();
  if (data.code && data.code !== 0) {
    const payload = {
      token: request.headers.Authorization,
      res: data,
      body: request.data,
      url: response.url
    };
  }
  // response 拦截
  if (data.code === 401 || data.Code === 401) {
    notification.error({
      message: data.errorMessage,
      description: data.errorMessage,
      duration: 3,
      top: 65
    });
    store.domainPrefix = '';
  } else if (data.code !== 0) {
    if (data.code === 100) {
      //code为100 时账户名或者密码错误, 为了跟之前弹窗一样所以用message
      message.error(data.errorMessage);
    } else {
      notification.error({
        message: `提示 : ${data.code}`,
        description: data.errorMessage,
        duration: 3,
        top: 65
      });
    }

    return response; //这里return response, 是为了避免modal里面报code undefined
  } else {
    return response;
  }
});

export default request;
