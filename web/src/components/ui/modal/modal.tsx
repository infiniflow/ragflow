// src/components/ui/modal.tsx
import { cn } from '@/lib/utils';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { Loader, X } from 'lucide-react';
import { FC, ReactNode, useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { createPortalModal } from './modal-manage';

export interface ModalProps {
  open: boolean;
  onOpenChange?: (open: boolean) => void;
  title?: ReactNode;
  titleClassName?: string;
  children: ReactNode;
  footer?: ReactNode;
  footerClassName?: string;
  showfooter?: boolean;
  className?: string;
  size?: 'small' | 'default' | 'large';
  closable?: boolean;
  closeIcon?: ReactNode;
  maskClosable?: boolean;
  destroyOnClose?: boolean;
  full?: boolean;
  confirmLoading?: boolean;
  cancelText?: ReactNode | string;
  okText?: ReactNode | string;
  onOk?: () => void;
  onCancel?: () => void;
  disabled?: boolean;
}
export interface ModalType extends FC<ModalProps> {
  show: typeof modalIns.show;
  hide: typeof modalIns.hide;
  destroy: typeof modalIns.destroy;
}

const Modal: ModalType = ({
  open,
  onOpenChange,
  title,
  titleClassName,
  children,
  footer,
  footerClassName,
  showfooter = true,
  className = '',
  size = 'default',
  closable = true,
  closeIcon = <X className="w-4 h-4" />,
  maskClosable = true,
  destroyOnClose = false,
  full = false,
  onOk,
  onCancel,
  confirmLoading,
  cancelText,
  okText,
  disabled = false,
}) => {
  const sizeClasses = {
    small: 'max-w-md',
    default: 'max-w-2xl',
    large: 'max-w-4xl',
  };

  const { t } = useTranslation();
  // Handle ESC key close
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && maskClosable) {
        onOpenChange?.(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [maskClosable, onOpenChange]);

  const handleCancel = useCallback(() => {
    onOpenChange?.(false);
    onCancel?.();
  }, [onCancel, onOpenChange]);

  const handleOk = useCallback(() => {
    onOpenChange?.(true);
    onOk?.();
  }, [onOk, onOpenChange]);
  const handleChange = (open: boolean) => {
    if (!open && !maskClosable) {
      return;
    }
    onOpenChange?.(open);
    console.log('open', open, onOpenChange);
    if (open && !disabled) {
      onOk?.();
    }
    if (!open) {
      onCancel?.();
    }
  };
  const footEl = useMemo(() => {
    if (showfooter === false) {
      return <></>;
    }
    let footerTemp;
    if (footer) {
      footerTemp = footer;
    } else {
      footerTemp = (
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={() => handleCancel()}
            className="px-2 py-1 border border-border-button rounded-md hover:bg-bg-card hover:text-text-primary "
          >
            {cancelText ?? t('modal.cancelText')}
          </button>
          <button
            type="button"
            disabled={confirmLoading || disabled}
            onClick={() => handleOk()}
            className={cn(
              'px-2 py-1 bg-primary text-primary-foreground rounded-md hover:bg-primary/90',
              { 'cursor-not-allowed': disabled },
            )}
          >
            {confirmLoading && (
              <Loader className="inline-block mr-2 h-4 w-4 animate-spin" />
            )}
            {okText ?? t('modal.okText')}
          </button>
        </div>
      );
    }
    return (
      <div
        className={cn(
          'flex items-center justify-end px-6 py-6',
          footerClassName,
        )}
      >
        {footerTemp}
      </div>
    );
  }, [
    disabled,
    footer,
    cancelText,
    t,
    confirmLoading,
    okText,
    handleCancel,
    handleOk,
    showfooter,
    footerClassName,
  ]);
  return (
    <DialogPrimitive.Root open={open} onOpenChange={handleChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-50 bg-colors-background-neutral-weak/50 backdrop-blur-sm flex items-center justify-center p-4"
          onClick={() => maskClosable && onOpenChange?.(false)}
        >
          <DialogPrimitive.Content
            className={`relative w-[700px] ${full ? 'max-w-full' : sizeClasses[size]} ${className} bg-bg-base rounded-lg shadow-lg border border-border-default transition-all focus-visible:!outline-none`}
            onClick={(e) => e.stopPropagation()}
          >
            {/* title */}
            {(title || closable) && (
              <div
                className={cn(
                  'flex items-center px-6 py-4',
                  {
                    'justify-end': closable && !title,
                    'justify-between': closable && title,
                    'justify-start': !closable,
                  },
                  titleClassName,
                )}
              >
                {title && (
                  <DialogPrimitive.Title className="text-lg font-medium text-foreground">
                    {title}
                  </DialogPrimitive.Title>
                )}
                {closable && (
                  <DialogPrimitive.Close asChild>
                    <button
                      type="button"
                      className="flex h-7 w-7 items-center justify-center rounded-full hover:bg-muted focus-visible:outline-none"
                      onClick={handleCancel}
                    >
                      {closeIcon}
                    </button>
                  </DialogPrimitive.Close>
                )}
              </div>
            )}

            {/* content */}
            <div className="py-2 px-6 overflow-y-auto scrollbar-auto max-h-[80vh] focus-visible:!outline-none">
              {destroyOnClose && !open ? null : children}
            </div>

            {/* footer */}
            {footEl}
          </DialogPrimitive.Content>
        </DialogPrimitive.Overlay>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
};

let modalIns = createPortalModal();
Modal.show = modalIns
  ? modalIns.show
  : () => {
      modalIns = createPortalModal();
      return modalIns.show;
    };
Modal.hide = modalIns.hide;
Modal.destroy = modalIns.destroy;

export { Modal };
