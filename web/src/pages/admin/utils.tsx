import {
  ColumnFilterAutoRemoveTestFn,
  FilterFn,
  Row,
  RowData,
  SortDirection,
  Table,
  TransformFilterValueFn,
} from '@tanstack/react-table';
import { LucideSortAsc, LucideSortDesc } from 'lucide-react';

export function parseBooleanish(value: any): boolean {
  return typeof value === 'string'
    ? /^(1|[Tt]rue|[Oo]n|[Yy](es)?)$/.test(value)
    : !!value;
}

export function createFuzzySearchFn<TData extends RowData>(
  columns: (keyof TData)[] = [],
) {
  return (row: Row<TData>, columnId: string, filterValue: string) => {
    const searchText = filterValue.trim().toLowerCase();

    return columns
      .map((n) =>
        row
          .getValue<string>(n as string)
          .trim()
          .toLowerCase(),
      )
      .some((v) => v.includes(searchText));
  };
}

export function createColumnFilterFn<TData extends RowData>(
  filterFn: FilterFn<TData>,
  options: {
    resolveFilterValue?: TransformFilterValueFn<TData>;
    autoRemove?: ColumnFilterAutoRemoveTestFn<TData>;
  },
) {
  return Object.assign(filterFn, options) as FilterFn<TData>;
}

export function getColumnFilter<TData extends RowData>(
  table: Table<TData>,
  columnId: string,
) {
  return table
    .getState()
    .columnFilters.find((filter) => filter.id === columnId);
}

export function setColumnFilter<TData extends RowData>(
  table: Table<TData>,
  columnId: string,
  value?: unknown,
) {
  const otherColumnFilters = table
    .getState()
    .columnFilters.filter((filter) => filter.id !== columnId);

  if (value == null) {
    table.setColumnFilters(otherColumnFilters);
  } else {
    table.setColumnFilters([
      ...otherColumnFilters,
      {
        id: columnId,
        value,
      },
    ]);
  }
}

export function getSortIcon(sorting: false | SortDirection) {
  return {
    asc: <LucideSortAsc />,
    desc: <LucideSortDesc />,
  }[sorting as string];
}

export const EMPTY_DATA = Object.freeze<any[]>([]) as any[];
export const IS_ENTERPRISE =
  process.env.UMI_APP_RAGFLOW_ENTERPRISE === 'RAGFLOW_ENTERPRISE';
