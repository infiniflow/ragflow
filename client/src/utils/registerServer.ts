const registerServer = (opt, request) => {
  let server = {};
  for (let key in opt) {
    server[key] = params => {
      if (opt[key].method === 'post' || opt[key].method === 'POST') {
        return request(opt[key].url, {
          method: opt[key].method,
          data: params
        });
      }

      if (opt[key].method === 'get' || opt[key].method === 'GET') {
        return request.get(opt[key].url, {
          params
        });
      }
    };
  }
  return server;
};

export default registerServer;
