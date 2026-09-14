import { ReactNode, useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { createRoot } from 'react-dom/client';
import { Modal, ModalProps } from './modal';

type PortalModalProps = Omit<ModalProps, 'open' | 'onOpenChange'> & {
  visible: boolean;
  onVisibleChange: (visible: boolean) => void;
  container?: HTMLElement;
  children: ReactNode;
  [key: string]: any;
};

const PortalModal = ({
  visible,
  onVisibleChange,
  container,
  children,
  ...restProps
}: PortalModalProps) => {
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
    return () => setMounted(false);
  }, []);

  if (!mounted || !visible) return null;
  console.log('PortalModal:', visible);
  return createPortal(
    <Modal open={visible} onOpenChange={onVisibleChange} {...restProps}>
      {children}
    </Modal>,
    container || document.body,
  );
};

export const createPortalModal = () => {
  let container = document.createElement('div');
  document.body.appendChild(container);

  let currentProps: any = {};
  let isVisible = false;
  let root: ReturnType<typeof createRoot> | null = null;

  root = createRoot(container);
  const destroy = () => {
    if (root && container) {
      root.unmount();
      if (container.parentNode) {
        container.parentNode.removeChild(container);
      }
      root = null;
    }
    isVisible = false;
    currentProps = {};
  };
  const render = () => {
    const { onVisibleChange, ...props } = currentProps;
    const modalParam = {
      visible: isVisible,

      onVisibleChange: (visible: boolean) => {
        isVisible = visible;
        if (onVisibleChange) {
          onVisibleChange(visible);
        }

        if (!visible) {
          render();
        }
      },
      ...props,
    };
    root?.render(isVisible ? <PortalModal {...modalParam} /> : null);
  };

  const show = (props: PortalModalProps) => {
    if (!container) {
      container = document.createElement('div');
      document.body.appendChild(container);
    }
    if (!root) {
      root = createRoot(container);
    }
    currentProps = { ...currentProps, ...props };
    isVisible = true;
    render();
  };

  const hide = () => {
    isVisible = false;
    render();
  };

  const update = (props = {}) => {
    currentProps = { ...currentProps, ...props };
    render();
  };

  return { show, hide, update, destroy };
};
