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
