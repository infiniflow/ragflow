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

import { t } from 'i18next';
import { Loader2 } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { TableCell, TableRow } from './ui/table';

type IProps = { columnsLength: number };

function Row({ children, columnsLength }: PropsWithChildren & IProps) {
  return (
    <TableRow>
      <TableCell colSpan={columnsLength} className="h-24 text-center">
        {children}
      </TableCell>
    </TableRow>
  );
}

export function TableSkeleton({
  columnsLength,
  children,
}: PropsWithChildren & IProps) {
  return (
    <Row columnsLength={columnsLength}>
      {children || (
        <Loader2 className="animate-spin size-16 inline-block text-gray-400" />
      )}
    </Row>
  );
}

export function TableEmpty({ columnsLength }: { columnsLength: number }) {
  return <Row columnsLength={columnsLength}>{t('common.noResults')}</Row>;
}
