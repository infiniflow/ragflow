/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

/* oxlint-disable guard-for-in */
import { AxiosRequestConfig, AxiosResponse } from 'axios';
import { isObject } from 'lodash';
import omit from 'lodash/omit';
import { RequestMethod } from 'umi-request';
import request from './next-request';

type Service<T extends string> = Record<
  T,
  (params?: any, urlAppendix?: string) => any
>;

const Methods = ['post', 'delete', 'put', 'patch'];

const registerServer = <T extends string>(
  opt: Record<T, { url: string; method: string }>,
  request: RequestMethod,
) => {
  const server: Service<T> = {} as Service<T>;
  for (const key in opt) {
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
