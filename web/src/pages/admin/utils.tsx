import {
  ColumnFilterAutoRemoveTestFn,
  FilterFn,
  Row,
  RowData,
  SortDirection,
  TransformFilterValueFn,
} from '@tanstack/react-table';
import { LucideSortAsc, LucideSortDesc } from 'lucide-react';
import { DefaultValues } from 'react-hook-form';

type AsyncDefaultValues<TValues> = (payload?: unknown) => Promise<TValues>;

export function formMergeDefaultValues<T>(
  ...parts: (DefaultValues<T> | AsyncDefaultValues<T> | undefined)[]
): (payload?: unknown) => Promise<Required<T>> {
  return async (payload?: unknown) => {
    if (parts.length === 0) {
      return {} as DefaultValues<T>;
    }

    if (parts.length === 1) {
      return typeof parts[0] === 'function'
        ? await parts[0](payload)
        : parts[0];
    }

    return Object.assign(
      // @ts-ignore
      ...(await Promise.all(
        parts.map((p) => (typeof p === 'function' ? p(payload) : p)),
      )),
    );
  };
}

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

export function getSortIcon(sorting: false | SortDirection) {
  return {
    asc: <LucideSortAsc />,
    desc: <LucideSortDesc />,
  }[sorting as string];
}

export const PERMISSION_TYPES = ['enable', 'read', 'write', 'share'] as const;
export const EMPTY_DATA = Object.freeze<any[]>([]) as any[];
export const IS_ENTERPRISE =
  import.meta.env.VITE_RAGFLOW_ENTERPRISE === 'RAGFLOW_ENTERPRISE';
