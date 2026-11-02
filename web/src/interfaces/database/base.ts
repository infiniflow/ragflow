export interface ResponseType<T extends object = any> {
  code: number;
  data: T | null;
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
