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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ButtonLoading } from '../ui/button';
import { RenameForm } from './rename-form';

export function RenameDialog({
  hideModal,
  initialName,
  onOk,
  loading,
  title,
  forbidSlash,
}: IModalProps<any> & {
  initialName?: string;
  title?: ReactNode;
  forbidSlash?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]" data-testid="rename-modal">
        <DialogHeader>
          <DialogTitle>{title || t('common.rename')}</DialogTitle>
        </DialogHeader>
        <RenameForm
          initialName={initialName}
          hideModal={hideModal}
          onOk={onOk}
          forbidSlash={forbidSlash}
        ></RenameForm>
        <DialogFooter>
          <ButtonLoading
            data-testid="rename-save"
            type="submit"
            form={TagRenameId}
            loading={loading}
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
