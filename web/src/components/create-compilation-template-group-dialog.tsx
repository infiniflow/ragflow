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
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { CompilationTemplateGroupEditor } from '@/components/compilation-template-group';
import { CompilationTemplateGroupSaveResult } from '@/components/compilation-template-group/hooks/use-compilation-template-group-submit';
import { CompilationTemplateGroupKeys } from '@/hooks/use-compilation-template-group-request';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

type CreateCompilationTemplateGroupDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (group: ICompilationTemplateGroup) => void;
};

export function CreateCompilationTemplateGroupDialog({
  open,
  onOpenChange,
  onCreated,
}: CreateCompilationTemplateGroupDialogProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const handleSuccess = useCallback(
    async (result: CompilationTemplateGroupSaveResult) => {
      // Land the new group in the options cache before the field adopts its
      // id, otherwise the select briefly renders it as a missing value.
      await queryClient.invalidateQueries({
        queryKey: CompilationTemplateGroupKeys.all(),
      });
      onOpenChange(false);
      if (result.data) {
        onCreated(result.data);
      }
    },
    [onCreated, onOpenChange, queryClient],
  );

  const handleCancel = useCallback(() => {
    onOpenChange(false);
  }, [onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-5xl h-[85vh] flex flex-col gap-0 p-0">
        <DialogHeader className="mx-0 mt-0 shrink-0 px-5 py-4">
          <DialogTitle>
            {t('knowledgeCompilation.addTemplateGroup')}
          </DialogTitle>
        </DialogHeader>
        <CompilationTemplateGroupEditor
          onSuccess={handleSuccess}
          onCancel={handleCancel}
          cancelText={t('common.cancel')}
        />
      </DialogContent>
    </Dialog>
  );
}
