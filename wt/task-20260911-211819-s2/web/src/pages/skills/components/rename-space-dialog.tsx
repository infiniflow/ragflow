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
import type { SkillSpace } from '../types';

interface RenameSpaceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  spaceToRename: SkillSpace | null;
  renameSpaceInput: string;
  onRenameInputChange: (value: string) => void;
  onRename: () => void;
}

export const RenameSpaceDialog: React.FC<RenameSpaceDialogProps> = ({
  open,
  onOpenChange,
  spaceToRename,
  renameSpaceInput,
  onRenameInputChange,
  onRename,
}) => {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>
            {t('skills.renameSpaceTitle') || 'Rename Skill Space'}
          </DialogTitle>
          <DialogDescription>
            {t('skills.renameSpaceDescription') ||
              'Enter a new name for this skill space.'}
          </DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <label className="text-sm font-medium mb-2 block">
            {t('skills.spaceName') || 'Space Name'}
          </label>
          <Input
            placeholder={t('skills.spaceNamePlaceholder') || 'e.g., my-space'}
            value={renameSpaceInput}
            onChange={(e) => onRenameInputChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && renameSpaceInput.trim()) {
                onRename();
              }
            }}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={onRename}
            disabled={
              !renameSpaceInput.trim() ||
              renameSpaceInput.trim() === spaceToRename?.name
            }
          >
            {t('common.save') || 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default RenameSpaceDialog;
