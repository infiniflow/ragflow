import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { DialogProps } from '@radix-ui/react-dialog';
import { useTranslation } from 'react-i18next';

interface IProps {
  title?: string;
  onOk?: (...args: any[]) => any;
  onCancel?: (...args: any[]) => any;
  hidden?: boolean;
}

export function ConfirmDeleteDialog({
  children,
  title,
  onOk,
  onCancel,
  hidden = false,
  onOpenChange,
  open,
  defaultOpen,
}: IProps & DialogProps) {
  const { t } = useTranslation();

  if (hidden) {
    return children;
  }

  return (
    <AlertDialog
      onOpenChange={onOpenChange}
      open={open}
      defaultOpen={defaultOpen}
    >
      <AlertDialogTrigger asChild>{children}</AlertDialogTrigger>
      <AlertDialogContent
        onSelect={(e) => e.preventDefault()}
        onClick={(e) => e.stopPropagation()}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>
            {title ?? t('common.deleteModalTitle')}
          </AlertDialogTitle>
          {/* <AlertDialogDescription>
            This action cannot be undone. This will permanently delete your
            account and remove your data from our servers.
          </AlertDialogDescription> */}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onCancel}>
            {t('common.no')}
          </AlertDialogCancel>
          <AlertDialogAction
            className="bg-state-error text-text-primary"
            onClick={onOk}
          >
            {t('common.yes')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
