import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import React from 'react';
import { useTranslation } from 'react-i18next';
import type { SkillSpace } from '../types';

interface DeleteSpaceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  spaceToDelete: SkillSpace | null;
  onDelete: () => void;
}

export const DeleteSpaceDialog: React.FC<DeleteSpaceDialogProps> = ({
  open,
  onOpenChange,
  spaceToDelete,
  onDelete,
}) => {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>
            {t('skills.deleteSpaceTitle') || 'Delete Skill Space'}
          </DialogTitle>
          <DialogDescription>
            {t('skills.deleteSpaceDescription') ||
              'Are you sure you want to delete this skill space? This action cannot be undone and all skills in this space will be permanently deleted.'}
          </DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <p className="text-sm text-text-secondary">
            {t('skills.deleteSpaceName') || 'Space name'}:{' '}
            <strong>{spaceToDelete?.name}</strong>
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button variant="destructive" onClick={onDelete}>
            {t('common.delete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default DeleteSpaceDialog;
