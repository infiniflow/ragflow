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
