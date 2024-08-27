import omit from 'lodash/omit';
import { RequestMethod } from 'umi-request';

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
