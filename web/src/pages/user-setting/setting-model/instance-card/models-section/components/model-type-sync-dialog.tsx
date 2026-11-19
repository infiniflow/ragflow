import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Loader2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { mapModelKey } from '../../available-models';
import type { ModelTypeChange } from '../hooks';

interface ModelTypeSyncDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  changes: ModelTypeChange[];
  result: { success: number; failed: number } | null;
  syncing: boolean;
  onConfirm: () => void;
}

function displayTypes(types: string[]) {
  return types
    .map((type) => mapModelKey[type as keyof typeof mapModelKey] || type)
    .join(', ');
}

export function ModelTypeSyncDialog({
  open,
  onOpenChange,
  changes,
  result,
  syncing,
  onConfirm,
}: ModelTypeSyncDialogProps) {
  const { t } = useTranslation();
  const handleCancel = useCallback(() => onOpenChange(false), [onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent closeDisabled={syncing}>
        <DialogHeader>
          <DialogTitle>{t('setting.syncModelTypes')}</DialogTitle>
          <DialogDescription>
            {t('setting.syncModelTypesDescription')}
          </DialogDescription>
        </DialogHeader>
        <ul className="max-h-80 overflow-auto divide-y divide-border-button rounded border border-border-button">
          {changes.map((change) => (
            <li key={change.name} className="space-y-1 p-3 text-sm">
              <p className="break-words font-medium">{change.name}</p>
              <p className="break-words text-text-secondary">
                {displayTypes(change.currentTypes)} →{' '}
                {displayTypes(change.nextTypes)}
              </p>
            </li>
          ))}
        </ul>
        {result && (
          <p role="status" className="break-words text-sm text-text-secondary">
            {t('setting.syncModelTypesPartial', result)}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={handleCancel} disabled={syncing}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={onConfirm}
            disabled={syncing || changes.length === 0}
            data-testid="models-confirm-type-sync"
          >
            {syncing && <Loader2 className="size-4 animate-spin" />}
            {t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
