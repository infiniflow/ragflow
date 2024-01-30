import { RequestMethod } from 'umi-request';

type Service<T extends string> = Record<T, (params: any) => any>;

const registerServer = <T extends string>(
  opt: Record<T, { url: string; method: string }>,
  request: RequestMethod,
) => {
  const server: Service<T> = {} as Service<T>;
  for (let key in opt) {
    server[key] = (params) => {
      if (opt[key].method === 'post' || opt[key].method === 'POST') {
        return request(opt[key].url, {
          method: opt[key].method,
          data: params,
        });
      }

      if (opt[key].method === 'get' || opt[key].method === 'GET') {
        return request.get(opt[key].url, {
          params,
        });
      }
    };
  }
  return server;
};

export default registerServer;
