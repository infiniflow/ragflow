/* eslint-disable guard-for-in */
import { AxiosRequestConfig, AxiosResponse } from 'axios';
import { isObject } from 'lodash';
import omit from 'lodash/omit';
import { RequestMethod } from 'umi-request';
import request from './next-request';

type Service<T extends string> = Record<
  T,
  (params?: any, urlAppendix?: string) => any
>;

const Methods = ['post', 'delete', 'put'];

const registerServer = <T extends string>(
  opt: Record<T, { url: string; method: string }>,
  request: RequestMethod,
) => {
  const server: Service<T> = {} as Service<T>;
  for (let key in opt) {
    server[key] = (params?: any, urlAppendix?: string) => {
      let url = opt[key].url;
      const requestOptions = opt[key];
      if (urlAppendix) {
        url = url + '/' + urlAppendix;
      }
      if (Methods.some((x) => x === opt[key].method.toLowerCase())) {
        return request(url, {
          method: opt[key].method,
          data: params,
        });
      }

      if (opt[key].method === 'get' || opt[key].method === 'GET') {
        return request.get(url, {
          ...omit(requestOptions, ['method', 'url']),
          params,
        });
      }
    };
  }
  return server;
};

export default registerServer;

export function registerNextServer<T extends string>(
  requestRecord: Record<
    T,
    { url: string | ((...args: Array<any>) => string); method: string }
  >,
) {
  type Server = Record<
    T,
    (
      config?:
        | AxiosRequestConfig<any>
        | Record<string, any>
        | string
        | number
        | boolean
        | undefined,
      useAxiosNativeConfig?: boolean,
    ) => Promise<AxiosResponse<any, any>>
  >;
  const server: Server = {} as Server;

  for (const name in requestRecord) {
    if (Object.prototype.hasOwnProperty.call(requestRecord, name)) {
      const { url, method } = requestRecord[name];
      server[name] = (config, useAxiosNativeConfig = false) => {
        const nextConfig = useAxiosNativeConfig ? config : { data: config };
        const finalConfig = isObject(nextConfig) ? nextConfig : {};
        const nextUrl = typeof url === 'function' ? url(config) : url;
        return request({ url: nextUrl, method, ...finalConfig });
      };
    }
  }

  return server;
}
