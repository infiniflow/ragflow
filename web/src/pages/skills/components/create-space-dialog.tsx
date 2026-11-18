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

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import React from 'react';
import { useTranslation } from 'react-i18next';

interface CreateSpaceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  spaceInput: string;
  onSpaceInputChange: (value: string) => void;
  onCreate: () => void;
}

export const CreateSpaceDialog: React.FC<CreateSpaceDialogProps> = ({
  open,
  onOpenChange,
  spaceInput,
  onSpaceInputChange,
  onCreate,
}) => {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>
            {t('skills.createSpaceTitle') || 'Create New Skill Space'}
          </DialogTitle>
          <DialogDescription>
            {t('skills.createSpaceDescription') ||
              'Create a new space to organize and manage your skills.'}
          </DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <label className="text-sm font-medium mb-2 block">
            {t('skills.spaceName') || 'Space Name'}
          </label>
          <Input
            placeholder={t('skills.spaceNamePlaceholder') || 'e.g., my-space'}
            value={spaceInput}
            onChange={(e) => onSpaceInputChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && spaceInput.trim()) {
                onCreate();
              }
            }}
          />
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              onOpenChange(false);
              onSpaceInputChange('');
            }}
          >
            {t('common.cancel')}
          </Button>
          <Button onClick={onCreate} disabled={!spaceInput.trim()}>
            {t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
