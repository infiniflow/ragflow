import '@tanstack/react-table';
declare module 'lodash';

declare global {
  type Nullable<T> = T | null;
}

declare module '@tanstack/react-table' {
  interface ColumnMeta {
    headerClassName?: string;
    cellClassName?: string;
  }
}
