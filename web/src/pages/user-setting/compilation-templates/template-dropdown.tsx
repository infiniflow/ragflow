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

import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { Trash2 } from 'lucide-react';
import { PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';

type TemplateDropdownProps = PropsWithChildren<{
  data: ICompilationTemplateGroup;
  onDelete: (id: string) => void;
}>;

export function TemplateDropdown({
  children,
  data,
  onDelete,
}: TemplateDropdownProps) {
  const { t } = useTranslation();

  const handleDelete = useCallback(() => {
    onDelete(data.id);
  }, [data.id, onDelete]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <div onClick={(e) => e.stopPropagation()}>{children}</div>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <ConfirmDeleteDialog
          title={t('setting.deleteTemplateGroupModalTitle')}
          content={{
            title: t('setting.deleteTemplateGroupModalContent'),
            node: <ConfirmDeleteDialogNode name={data.name} />,
          }}
          onOk={handleDelete}
        >
          <DropdownMenuItem
            className="text-state-error"
            onSelect={(e) => e.preventDefault()}
            onClick={(e) => e.stopPropagation()}
          >
            {t('common.delete')}
            <Trash2 className="size-4" />
          </DropdownMenuItem>
        </ConfirmDeleteDialog>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
