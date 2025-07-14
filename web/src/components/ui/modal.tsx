// src/components/ui/modal.tsx
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { Loader, X } from 'lucide-react';
import * as React from 'react';
import { useTranslation } from 'react-i18next';

interface ModalProps {
  open: boolean;
  onOpenChange?: (open: boolean) => void;
  title?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  className?: string;
  size?: 'small' | 'default' | 'large';
  closable?: boolean;
  closeIcon?: React.ReactNode;
  maskClosable?: boolean;
  destroyOnClose?: boolean;
  full?: boolean;
  confirmLoading?: boolean;
  cancelText?: React.ReactNode | string;
  okText?: React.ReactNode | string;
  onOk?: () => void;
  onCancel?: () => void;
}

export const Modal: React.FC<ModalProps> = ({
  open,
  onOpenChange,
  title,
  children,
  footer,
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
}) => {
  const sizeClasses = {
    small: 'max-w-md',
    default: 'max-w-2xl',
    large: 'max-w-4xl',
  };

  const { t } = useTranslation();
  // Handle ESC key close
  React.useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && maskClosable) {
        onOpenChange?.(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [maskClosable, onOpenChange]);

  const handleCancel = () => {
    onOpenChange?.(false);
    onCancel?.();
  };
  const handleOk = () => {
    onOpenChange?.(true);
    onOk?.();
  };
  const handleChange = (open: boolean) => {
    onOpenChange?.(open);
    if (open) {
      handleOk();
    }
    if (!open) {
      handleCancel();
    }
  };
  const footEl = React.useMemo(() => {
    let footerTemp;
    if (footer) {
      footerTemp = footer;
    } else {
      footerTemp = (
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={() => handleCancel()}
            className="px-2 py-1 border border-input rounded-md hover:bg-muted"
          >
            {cancelText ?? t('modal.cancelText')}
          </button>
          <button
            type="button"
            disabled={confirmLoading}
            onClick={() => handleOk()}
            className="px-2 py-1 bg-primary text-primary-foreground rounded-md hover:bg-primary/90"
          >
            {confirmLoading && (
              <Loader className="inline-block mr-2 h-4 w-4 animate-spin" />
            )}
            {okText ?? t('modal.okText')}
          </button>
        </div>
      );
      return (
        <div className="flex items-center justify-end border-t border-border px-6 py-4">
          {footerTemp}
        </div>
      );
    }
  }, [footer, confirmLoading, onOpenChange, onCancel, onOk]);
  return (
    <DialogPrimitive.Root open={open} onOpenChange={handleChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-50 bg-black/30 backdrop-blur-sm flex items-center justify-center p-4"
          onClick={() => maskClosable && onOpenChange?.(false)}
        >
          <DialogPrimitive.Content
            className={`relative w-[500px] ${full ? 'max-w-full' : sizeClasses[size]} ${className} bg-background rounded-lg shadow-lg transition-all`}
            onClick={(e) => e.stopPropagation()}
          >
            {/* title */}
            {title && (
              <div className="flex items-center justify-between border-b border-border px-6 py-4">
                <DialogPrimitive.Title className="text-lg font-medium text-foreground">
                  {title}
                </DialogPrimitive.Title>
                {closable && (
                  <DialogPrimitive.Close asChild>
                    <button
                      type="button"
                      className="flex h-7 w-7 items-center justify-center rounded-full hover:bg-muted"
                    >
                      {closeIcon}
                    </button>
                  </DialogPrimitive.Close>
                )}
              </div>
            )}

            {/* content */}
            <div className="p-6 overflow-y-auto max-h-[80vh]">
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

// example usage
/*
import { Modal } from '@/components/ui/modal';

function Demo() {
  const [open, setOpen] = useState(false);

  return (
    <div>
      <button onClick={() => setOpen(true)}>open modal</button>
      
      <Modal
        open={open}
        onOpenChange={setOpen}
        title="title"
        footer={
          <div className="flex gap-2">
            <button onClick={() => setOpen(false)} className="px-4 py-2 border rounded-md">
              cancel
            </button>
            <button onClick={() => setOpen(false)} className="px-4 py-2 bg-primary text-white rounded-md">
              ok
            </button>
          </div>
        }
      >
        <div className="py-4">弹窗内容区域</div>
      </Modal>
    </div>
  );
}
*/
