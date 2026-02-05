import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { AlertDialogOverlay } from '@radix-ui/react-alert-dialog';
import { DialogProps } from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { RAGFlowAvatar } from './ragflow-avatar';
import { Separator } from './ui/separator';

interface IProps {
  title?: string;
  onOk?: (...args: any[]) => any;
  onCancel?: (...args: any[]) => any;
  hidden?: boolean;
  content?: {
    title?: string;
    node?: React.ReactNode;
  };
  okButtonText?: string;
  cancelButtonText?: string;
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
  content,
  okButtonText,
  cancelButtonText,
}: IProps & DialogProps) {
  const { t } = useTranslation();

  if (hidden) {
    return children || <></>;
  }

  return (
    <AlertDialog
      onOpenChange={onOpenChange}
      open={open}
      defaultOpen={defaultOpen}
    >
      {children && <AlertDialogTrigger asChild>{children}</AlertDialogTrigger>}
      <AlertDialogOverlay
        onClick={(e) => {
          e.stopPropagation();
        }}
      >
        <AlertDialogContent
          onSelect={(e) => e.preventDefault()}
          onClick={(e) => e.stopPropagation()}
          className="bg-bg-base "
        >
          <AlertDialogHeader className="space-y-5">
            <AlertDialogTitle>
              {title ?? t('common.deleteModalTitle')}
              <AlertDialogCancel
                onClick={onCancel}
                className="border-none bg-transparent hover:border-none hover:bg-transparent absolute right-3 top-3 hover:text-text-primary"
              >
                <X size={16} />
              </AlertDialogCancel>
            </AlertDialogTitle>
            {content && (
              <>
                <Separator className="w-[calc(100%+48px)] -translate-x-6"></Separator>
                <AlertDialogDescription className="mt-5">
                  <div className="flex flex-col gap-5  text-base mb-10 px-5">
                    <div className="text-text-primary">
                      {content.title || t('common.deleteModalTitle')}
                    </div>
                    {content.node}
                  </div>
                </AlertDialogDescription>
              </>
            )}
          </AlertDialogHeader>
          <AlertDialogFooter className="px-5 flex items-center gap-2">
            <AlertDialogCancel onClick={onCancel}>
              {cancelButtonText || t('common.cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              className="bg-state-error text-text-primary hover:text-text-primary hover:bg-state-error"
              onClick={onOk}
            >
              {okButtonText || t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialogOverlay>
    </AlertDialog>
  );
}

export const ConfirmDeleteDialogNode = ({
  avatar,
  name,
  warnText,
  children,
}: {
  avatar?: { avatar?: string; name?: string; isPerson?: boolean };
  name?: string;
  warnText?: string;
  children?: React.ReactNode;
}) => {
  return (
    <div className="flex flex-col gap-2.5">
      {(avatar || name) && (
        <div className="flex items-center border-0.5 text-text-secondary border-border-button rounded-lg px-3 py-4">
          {avatar && (
            <RAGFlowAvatar
              className="w-8 h-8"
              avatar={avatar.avatar}
              isPerson={avatar.isPerson}
              name={avatar.name}
            />
          )}
          {name && <div className="ml-3">{name}</div>}
        </div>
      )}
      {warnText && <div className="text-state-error text-xs">{warnText}</div>}
      {children}
    </div>
  );
};
