import { Button } from '@/components/ui/button';
import { Modal, ModalType } from '@/components/ui/modal/modal';
import { t } from 'i18next';
import { DataSourceInfo } from '../contant';
import { IDataSourceBase } from '../interface';

export type IDelSourceModalProps<T> = Partial<ModalType> & {
  data?: T;
  type?: 'delete' | 'unlink';
  onOk?: (data?: T) => void;
};

export const delSourceModal = <T extends IDataSourceBase>(
  props: IDelSourceModalProps<T>,
) => {
  const { data, onOk, type = 'delete', ...otherProps } = props;
  console.log('data', data);
  const config = {
    title:
      type === 'delete'
        ? t('setting.deleteSourceModalTitle')
        : t('dataflowParser.unlinkSourceModalTitle'),
    content: (
      <div className="px-2 py-6">
        <div className="flex items-center gap-1 p-2 border border-border-button rounded-md mb-3">
          <div className="w-6 h-6 flex-shrink-0">
            {data?.source ? DataSourceInfo[data?.source].icon : ''}
          </div>
          <div>{data?.name}</div>
        </div>
        {type === 'delete' ? (
          <div
            className="text-sm text-text-secondary"
            dangerouslySetInnerHTML={{
              __html: t('setting.deleteSourceModalContent'),
            }}
          ></div>
        ) : (
          <div
            className="text-sm text-text-secondary"
            dangerouslySetInnerHTML={{
              __html: t('dataflowParser.unlinkSourceModalContent'),
            }}
          />
        )}
      </div>
    ),
    confirmText:
      type === 'delete'
        ? t('setting.deleteSourceModalConfirmText')
        : t('dataflowParser.unlinkSourceModalConfirmText'),
  };
  Modal.show({
    visible: true,
    className: '!w-[560px]',
    ...otherProps,
    title: config.title,
    children: config.content,
    onVisibleChange: () => {
      Modal.hide();
    },
    footer: (
      <div className="flex justify-end gap-2">
        <Button variant={'outline'} onClick={() => Modal.hide()}>
          {t('dataflowParser.changeStepModalCancelText')}
        </Button>
        <Button
          variant={'secondary'}
          className="!bg-state-error text-text-base"
          onClick={() => {
            onOk?.(data);
            Modal.hide();
          }}
        >
          {config.confirmText}
        </Button>
      </div>
    ),
  });
};
