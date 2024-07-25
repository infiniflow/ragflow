export interface ResponseType<T = any> {
  retcode: number;
  data: T;
  retmsg: string;
  status: number;
}

export interface ResponseGetType<T = any> {
  data: T;
  loading?: boolean;
}
