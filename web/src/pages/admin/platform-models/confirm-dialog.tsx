import { ReactNode, useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { BusyButton } from './busy-button';
import { useReturnFocusOnClose, useSingleFlight } from './hooks';

interface PlatformConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description: ReactNode;
  /** The affected object, shown below the description. */
  subject?: ReactNode;
  confirmText: ReactNode;
  loading?: boolean;
  onConfirm: () => unknown;
}

/** Confirmation for platform changes that affect every tenant. */
export function PlatformConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  subject,
  confirmText,
  loading,
  onConfirm,
}: PlatformConfirmDialogProps) {
  const { t } = useTranslation();
  const returnFocus = useReturnFocusOnClose(open);
  // While the change runs the dialog stays open, so a failure is shown where
  // the admin made the change.
  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next && loading) return;
      onOpenChange(next);
    },
    [loading, onOpenChange],
  );
  // One change at a time, even for two clicks in the same frame.
  const handleConfirm = useSingleFlight(
    useCallback(async () => {
      await onConfirm();
    }, [onConfirm]),
  );
  const handleCancel = useCallback(
    () => handleOpenChange(false),
    [handleOpenChange],
  );
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent onCloseAutoFocus={returnFocus}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>

        <section className="px-6">
          <DialogDescription>{description}</DialogDescription>
          {subject && (
            <div className="rounded-lg mt-6 p-4 border-0.5 border-border-button min-w-0 [overflow-wrap:anywhere]">
              {subject}
            </div>
          )}
        </section>

        <DialogFooter className="gap-4 px-6 py-4">
          <Button
            className="px-4 h-10 dark:border-border-button"
            variant="outline"
            onClick={handleCancel}
            disabled={loading}
          >
            {t('admin.cancel')}
          </Button>
          <BusyButton
            className="px-4 h-10"
            variant="destructive"
            onClick={handleConfirm}
            busy={loading}
          >
            {confirmText}
          </BusyButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
