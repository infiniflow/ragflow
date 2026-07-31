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

interface DeleteSelectedSpacesDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedCount: number;
  onDelete: () => void;
}

export const DeleteSelectedSpacesDialog: React.FC<
  DeleteSelectedSpacesDialogProps
> = ({ open, onOpenChange, selectedCount, onDelete }) => {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>
            {t('skills.deleteSelectedTitle') || 'Delete Selected Spaces'}
          </DialogTitle>
          <DialogDescription>
            {t('skills.deleteSelectedDescription', { count: selectedCount }) ||
              `Are you sure you want to delete ${selectedCount} selected spaces? This action cannot be undone.`}
          </DialogDescription>
        </DialogHeader>
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

export default DeleteSelectedSpacesDialog;
