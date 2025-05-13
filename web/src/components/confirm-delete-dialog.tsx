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
import { Trash2 } from 'lucide-react';
import { PropsWithChildren } from 'react';
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
}: IProps & PropsWithChildren) {
  const { t } = useTranslation();

  if (hidden) {
    return children;
  }

  return (
    <AlertDialog>
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
            {t('common.cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            className="bg-colors-background-functional-solid-danger text--colors-text-neutral-strong"
            onClick={onOk}
          >
            <Trash2 />
            {t('common.ok')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
