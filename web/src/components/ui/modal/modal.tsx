// src/components/ui/modal.tsx
import { cn } from '@/lib/utils';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { AlertCircle, CheckCircle, Info, Loader, X } from 'lucide-react';
import { FC, ReactNode, useCallback, useEffect, useMemo } from 'react';
import { createRoot } from 'react-dom/client';
import { useTranslation } from 'react-i18next';
import { DialogDescription } from '../dialog';
import { createPortalModal } from './modal-manage';

export interface ModalProps {
  open: boolean;
  onOpenChange?: (open: boolean) => void;
  title?: ReactNode;
  titleClassName?: string;
  children?: ReactNode;
  content?: ReactNode;
  footer?: ReactNode;
  footerClassName?: string;
  showfooter?: boolean;
  className?: string;
  size?: 'small' | 'default' | 'large';
  closable?: boolean;
  showCancel?: boolean;
  closeIcon?: ReactNode;
  maskClosable?: boolean;
  destroyOnClose?: boolean;
  full?: boolean;
  confirmLoading?: boolean;
  cancelText?: ReactNode | string;
  okText?: ReactNode | string;
  onOk?: () => void;
  onCancel?: () => void;
  okButtonClassName?: string;
  cancelButtonClassName?: string;
  testId?: string;
  okButtonTestId?: string;
  disabled?: boolean;
  style?: React.CSSProperties;
  zIndex?: number;
  type?: 'warning' | 'info' | 'success' | 'error' | 'confirm';
}

export interface ModalType extends FC<ModalProps> {
  show: typeof modalIns.show;
  hide: typeof modalIns.hide;
  destroy: typeof modalIns.destroy;
  warning: (props: Omit<ModalProps, 'open' | 'type'>) => void;
  info: (props: Omit<ModalProps, 'open' | 'type'>) => void;
  success: (props: Omit<ModalProps, 'open' | 'type'>) => void;
  error: (props: Omit<ModalProps, 'open' | 'type'>) => void;
  confirm: (props: Omit<ModalProps, 'open' | 'type'>) => void;
}

const typeIcons = {
  warning: <AlertCircle className="w-6 h-6 text-yellow-500" />,
  info: <Info className="w-6 h-6 text-blue-500" />,
  success: <CheckCircle className="w-6 h-6 text-green-500" />,
  error: <AlertCircle className="w-6 h-6 text-red-500" />,
  confirm: <AlertCircle className="w-6 h-6 text-yellow-500" />,
};

const Modal: ModalType = ({
  open,
  onOpenChange,
  title,
  titleClassName,
  children,
  content,
  footer,
  footerClassName,
  showfooter = true,
  className = '',
  size = 'default',
  closable = true,
  showCancel = true,
  closeIcon = <X className="w-4 h-4" />,
  maskClosable = true,
  destroyOnClose = false,
  full = false,
  onOk,
  onCancel,
  confirmLoading,
  cancelText,
  okText,
  okButtonClassName,
  cancelButtonClassName,
  disabled = false,
  style,
  zIndex = 50,
  type,
  testId,
  okButtonTestId,
}) => {
  const sizeClasses = {
    small: 'max-w-md',
    default: 'max-w-2xl',
    large: 'max-w-4xl',
  };

  const { t } = useTranslation();
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
          {showCancel && (
            <button
              type="button"
              onClick={() => handleCancel()}
              className={cn(
                'px-2 py-1 border border-border-button rounded-md hover:bg-bg-card hover:text-text-primary ',
                cancelButtonClassName,
              )}
            >
              {cancelText ?? t('modal.cancelText')}
            </button>
          )}
          <button
            type="button"
            disabled={confirmLoading || disabled}
            onClick={() => handleOk()}
            data-testid={okButtonTestId}
            className={cn(
              'px-2 py-1 bg-primary text-primary-foreground rounded-md hover:bg-primary/90',
              { 'cursor-not-allowed': disabled },
              okButtonClassName,
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
    okButtonClassName,
    cancelButtonClassName,
    showCancel,
    okButtonTestId,
  ]);

  const contentEl = useMemo(() => {
    if (type && typeIcons[type]) {
      return (
        <div className="flex items-start gap-3">
          <div className="flex-shrink-0 mt-0.5">{typeIcons[type]}</div>
          <div className="flex-1">
            {title && (
              <DialogPrimitive.Title className="text-lg font-medium text-foreground mb-2">
                {title}
              </DialogPrimitive.Title>
            )}
            <div className="text-text-secondary">{content || children}</div>
          </div>
        </div>
      );
    }
    return children;
  }, [type, title, content, children]);

  return (
    <DialogPrimitive.Root open={open} onOpenChange={handleChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-[1000] bg-bg-card backdrop-blur-[1px] flex items-center justify-center p-4"
          onClick={() => maskClosable && onOpenChange?.(false)}
          style={{ zIndex: zIndex }}
        >
          <DialogPrimitive.Content
            className={cn(
              `relative w-[700px] ${full ? 'max-w-full' : sizeClasses[size]} ${className} bg-bg-base rounded-lg shadow-lg border border-border-default transition-all focus-visible:!outline-none`,
              { 'pt-10': closable && !title && !type },
            )}
            data-testid={testId}
            style={style}
            onClick={(e) => e.stopPropagation()}
          >
            <DialogDescription></DialogDescription>
            {title && !type && (
              <div
                className={cn(
                  'flex items-start px-6 py-4 justify-start',
                  titleClassName,
                )}
              >
                {title && (
                  <DialogPrimitive.Title className="text-lg font-medium text-foreground w-full">
                    {title}
                  </DialogPrimitive.Title>
                )}
              </div>
            )}
            {closable && (
              <DialogPrimitive.Close asChild>
                <button
                  type="button"
                  className="flex absolute right-5 top-5 h-7 w-7 items-center justify-center text-text-secondary rounded-full hover:text-text-primary focus-visible:outline-none"
                  onClick={handleCancel}
                >
                  {closeIcon}
                </button>
              </DialogPrimitive.Close>
            )}

            <div className="py-2 px-6 overflow-y-auto scrollbar-auto max-h-[calc(100vh-280px)] focus-visible:!outline-none">
              {destroyOnClose && !open ? null : contentEl}
            </div>

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

const createStaticModal = (type: ModalProps['type']) => {
  return (props: Omit<ModalProps, 'open' | 'type'>) => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    const destroy = () => {
      root.unmount();
      container.remove();
    };

    const handleOk = () => {
      props.onOk?.();
      destroy();
    };

    const handleCancel = () => {
      props.onCancel?.();
      destroy();
    };

    root.render(
      <Modal
        {...props}
        open={true}
        type={type}
        onOk={handleOk}
        onCancel={handleCancel}
        onOpenChange={(open) => {
          if (!open) {
            handleCancel();
          }
        }}
        maskClosable={false}
      />,
    );
  };
};

Modal.warning = createStaticModal('warning');
Modal.info = createStaticModal('info');
Modal.success = createStaticModal('success');
Modal.error = createStaticModal('error');
Modal.confirm = createStaticModal('confirm');

export { Modal };
