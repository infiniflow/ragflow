export interface Pagination {
  current: number;
  pageSize: number;
}

export interface BaseState {
  pagination: Pagination;
  searchString: string;
}
