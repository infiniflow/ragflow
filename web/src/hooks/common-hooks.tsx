import { Modal } from '@/components/ui/modal/modal';
import isEqual from 'lodash/isEqual';
import { ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const useSetModalState = (initialVisible = false) => {
  const [visible, setVisible] = useState(initialVisible);

  const showModal = useCallback(() => {
    setVisible(true);
  }, []);
  const hideModal = useCallback(() => {
    setVisible(false);
  }, []);

  const switchVisible = useCallback(() => {
    setVisible(!visible);
  }, [visible]);

  return { visible, showModal, hideModal, switchVisible };
};

export const useDeepCompareEffect = (
  effect: React.EffectCallback,
  deps: React.DependencyList,
) => {
  const ref = useRef<React.DependencyList>();
  let callback: ReturnType<React.EffectCallback> = () => {};
  if (!isEqual(deps, ref.current)) {
    callback = effect();
    ref.current = deps;
  }
  useEffect(() => {
    return () => {
      if (callback) {
        callback();
      }
    };
  }, []);
};

export interface UseDynamicSVGImportOptions {
  onCompleted?: (
    name: string,
    SvgIcon: React.FC<React.SVGProps<SVGSVGElement>> | undefined,
  ) => void;
  onError?: (err: Error) => void;
}

export function useDynamicSVGImport(
  name: string,
  options: UseDynamicSVGImportOptions = {},
) {
  const ImportedIconRef = useRef<React.FC<React.SVGProps<SVGSVGElement>>>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error>();

  const { onCompleted, onError } = options;
  useEffect(() => {
    setLoading(true);
    const importIcon = async (): Promise<void> => {
      try {
        ImportedIconRef.current = (
          await import(/* @vite-ignore */ name)
        ).ReactComponent;
        onCompleted?.(name, ImportedIconRef.current);
      } catch (err: any) {
        onError?.(err);
        setError(err);
      } finally {
        setLoading(false);
      }
    };
    importIcon();
  }, [name, onCompleted, onError]);

  return { error, loading, SvgIcon: ImportedIconRef.current };
}

interface IProps {
  header?: string | ReactNode;
  title?: string;
  content?: ReactNode;
  onOk?: (...args: any[]) => any;
  onCancel?: (...args: any[]) => any;
}

export const useShowDeleteConfirm = () => {
  const { t } = useTranslation();
  const showDeleteConfirm = useCallback(
    ({ title, content, onOk, onCancel, header }: IProps): Promise<number> => {
      return new Promise((resolve, reject) => {
        Modal.show({
          title: header,
          closable: !!header,
          visible: true,
          onVisibleChange: () => {
            Modal.destroy();
          },
          footer: null,
          maskClosable: false,
          okText: t('common.delete'),
          cancelText: t('common.cancel'),
          style: {
            width: '450px',
          },
          zIndex: 1000,
          okButtonClassName:
            'bg-state-error text-white hover:bg-state-error hover:text-white',
          onOk: async () => {
            try {
              const ret = await onOk?.();
              resolve(ret);
              console.info(ret);
            } catch (error) {
              reject(error);
            }
          },
          onCancel: () => {
            onCancel?.();
            Modal.destroy();
          },
          children: (
            <div className="flex flex-col justify-start items-start mt-3">
              <div className="text-lg font-medium">
                {title ?? t('common.deleteModalTitle')}
              </div>
              <div className="text-base font-normal">{content}</div>
            </div>
          ),
        });
      });
    },
    [t],
  );

  return showDeleteConfirm;
};

export const useTranslate = (keyPrefix: string) => {
  return useTranslation('translation', { keyPrefix });
};

export const useCommonTranslation = () => {
  return useTranslation('translation', { keyPrefix: 'common' });
};
