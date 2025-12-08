export interface Pagination {
  current: number;
  pageSize: number;
  total: number;
  onChange?: (page: number, pageSize: number) => void;
}

export interface BaseState {
  pagination: Pagination;
  searchString: string;
}

export interface IModalProps<T> {
  showModal?(): void;
  hideModal?(): void;
  switchVisible?(visible: boolean): void;
  visible?: boolean;
  loading?: boolean;
  onOk?(payload?: T): Promise<any> | void;
  initialValues?: T;
}
