import 'umi/typings';
declare module 'lodash';

// declare type Nullable<T> = T | null; invalid

declare global {
  type Nullable<T> = T | null;
}
