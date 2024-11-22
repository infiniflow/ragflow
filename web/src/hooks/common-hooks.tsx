import { ExclamationCircleFilled } from '@ant-design/icons';
import { App } from 'antd';
import isEqual from 'lodash/isEqual';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const useSetModalState = () => {
  const [visible, setVisible] = useState(false);

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
        ImportedIconRef.current = (await import(name)).ReactComponent;
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
  title?: string;
  onOk?: (...args: any[]) => any;
  onCancel?: (...args: any[]) => any;
}

export const useShowDeleteConfirm = () => {
  const { modal } = App.useApp();
  const { t } = useTranslation();

  const showDeleteConfirm = useCallback(
    ({ title, onOk, onCancel }: IProps): Promise<number> => {
      return new Promise((resolve, reject) => {
        modal.confirm({
          title: title ?? t('common.deleteModalTitle'),
          icon: <ExclamationCircleFilled />,
          // content: 'Some descriptions',
          okText: t('common.ok'),
          okType: 'danger',
          cancelText: t('common.cancel'),
          async onOk() {
            try {
              const ret = await onOk?.();
              resolve(ret);
              console.info(ret);
            } catch (error) {
              reject(error);
            }
          },
          onCancel() {
            onCancel?.();
          },
        });
      });
    },
    [t, modal],
  );

  return showDeleteConfirm;
};

export const useTranslate = (keyPrefix: string) => {
  return useTranslation('translation', { keyPrefix });
};

export const useCommonTranslation = () => {
  return useTranslation('translation', { keyPrefix: 'common' });
};
