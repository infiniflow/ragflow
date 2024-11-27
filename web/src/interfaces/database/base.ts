export interface ResponseType<T = any> {
  code: number;
  data: T;
  message: string;
  status: number;
}

export interface ResponseGetType<T = any> {
  data: T;
  loading?: boolean;
}

export interface ResponsePostType<T = any> {
  data: T;
  loading?: boolean;
  [key: string]: unknown;
}
