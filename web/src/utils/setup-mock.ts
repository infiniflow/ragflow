import debug from './env';

export default ({ mock, setup }: { mock?: boolean; setup(): void }) => {
  if ((mock !== false && debug) || import.meta.env.VITE_FORCE_MOCK) {
    setup();
  }
};

export const successResponseWrap = (data: unknown) => {
  return {
    data,
    status: 'ok',
    msg: '请求成功',
    code: 200,
  };
};

export const failResponseWrap = (data: unknown, msg: string, code = 500) => {
  return {
    data,
    status: 'fail',
    msg,
    code,
  };
};
