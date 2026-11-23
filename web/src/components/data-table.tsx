/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { flexRender, type Table as TanStackTable } from '@tanstack/react-table';
import type { ReactNode } from 'react';
import { TableEmpty } from './table-skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from './ui/table';

type DataTableProps<TData> = {
  table: TanStackTable<TData>;
  columnsLength: number;
  rootClassName?: string;
  bodyClassName?: string;
  rowClassName?: string;
  emptyContent?: ReactNode;
  applyColumnCellClassName?: boolean;
};

export function DataTable<TData>({
  table,
  columnsLength,
  rootClassName,
  bodyClassName,
  rowClassName,
  emptyContent,
  applyColumnCellClassName,
}: DataTableProps<TData>) {
  const rows = table.getRowModel().rows;

  return (
    <Table rootClassName={rootClassName}>
      <TableHeader>
        {table.getHeaderGroups().map((headerGroup) => (
          <TableRow key={headerGroup.id}>
            {headerGroup.headers.map((header) => (
              <TableHead key={header.id}>
                {header.isPlaceholder
                  ? null
                  : flexRender(
                      header.column.columnDef.header,
                      header.getContext(),
                    )}
              </TableHead>
            ))}
          </TableRow>
        ))}
      </TableHeader>
      <TableBody className={bodyClassName}>
        {rows.length ? (
          rows.map((row) => (
            <TableRow
              key={row.id}
              data-state={row.getIsSelected() && 'selected'}
              className={rowClassName}
            >
              {row.getVisibleCells().map((cell) => (
                <TableCell
                  key={cell.id}
                  className={
                    applyColumnCellClassName
                      ? cell.column.columnDef.meta?.cellClassName
                      : undefined
                  }
                >
                  {flexRender(cell.column.columnDef.cell, cell.getContext())}
                </TableCell>
              ))}
            </TableRow>
          ))
        ) : emptyContent === undefined ? (
          <TableEmpty columnsLength={columnsLength} />
        ) : (
          <TableRow>
            <TableCell colSpan={columnsLength} className="h-24 text-center">
              {emptyContent}
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  );
}
