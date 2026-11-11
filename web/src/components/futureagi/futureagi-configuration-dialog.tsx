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

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { useDeleteFutureAGIConfig } from '@/hooks/use-user-setting-request';
import { IModalProps } from '@/interfaces/common';
import { ExternalLink, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  FormId,
  FutureAGIConfigurationForm,
} from './futureagi-configuration-form';

export function FutureAGIConfigurationDialog({
  hideModal,
  loading,
  onOk,
}: IModalProps<any>) {
  const { t } = useTranslation();
  const { deleteFutureAGIConfig } = useDeleteFutureAGIConfig();

  const handleDelete = useCallback(async () => {
    const ret = await deleteFutureAGIConfig();
    if (ret === 0) {
      hideModal?.();
    }
  }, [deleteFutureAGIConfig, hideModal]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('setting.configuration')} Future AGI</DialogTitle>
          <DialogDescription>
            {t('setting.futureAGIDescription')}
          </DialogDescription>
        </DialogHeader>
        <FutureAGIConfigurationForm onOk={onOk}></FutureAGIConfigurationForm>
        <DialogFooter className="!justify-between">
          <a
            href="https://docs.futureagi.com/docs/integrations/no-code/ragflow"
            className="flex items-center gap-2 underline text-text-secondary hover:text-text-primary"
            target="_blank"
            rel="noreferrer"
          >
            {t('setting.viewFutureAGIDocumentation')}
            <ExternalLink className="size-4" />
          </a>
          <div className="flex items-center gap-4">
            <ConfirmDeleteDialog onOk={handleDelete}>
              <Button variant={'outline'}>
                <Trash2 /> {t('common.delete')}
              </Button>
            </ConfirmDeleteDialog>

            <ButtonLoading type="submit" form={FormId} loading={loading}>
              {t('common.save')}
            </ButtonLoading>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
