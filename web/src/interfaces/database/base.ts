export interface ResponseType<T = any> {
  retcode: number;
  data: T;
  retmsg: string;
  status: number;
}
