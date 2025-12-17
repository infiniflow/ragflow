import { Button } from '@/components/ui/button';
import { Modal, ModalType } from '@/components/ui/modal/modal';
import { t } from 'i18next';
import { IDataSourceBase, IDataSourceInfoMap } from '../interface';

export type IDelSourceModalProps<T> = Partial<ModalType> & {
  data?: T;
  type?: 'delete' | 'unlink';
  onOk?: (data?: T) => void;
  dataSourceInfo: IDataSourceInfoMap;
};

export const delSourceModal = <T extends IDataSourceBase>(
  props: IDelSourceModalProps<T>,
) => {
  const { data, onOk, type = 'delete', dataSourceInfo, ...otherProps } = props;
  const config = {
    title:
      type === 'delete'
        ? t('setting.deleteSourceModalTitle')
        : t('dataflowParser.unlinkSourceModalTitle'),
    content: (
      <div className="px-2 space-y-6 pt-5 pb-3">
        {type === 'delete' ? (
          <div
            className="text-base text-text-primary"
            dangerouslySetInnerHTML={{
              __html: t('setting.deleteSourceModalContent'),
            }}
          ></div>
        ) : (
          <div
            className="text-base text-text-primary"
            dangerouslySetInnerHTML={{
              __html: t('dataflowParser.unlinkSourceModalContent'),
            }}
          />
        )}
        <div className="flex items-center gap-1 p-2 border border-border-button rounded-md mb-3">
          <div className="w-6 h-6 flex-shrink-0">
            {data?.source ? dataSourceInfo[data?.source].icon : ''}
          </div>
          <div className="flex items-center gap-2 text-text-secondary text-xs">
            {/* <div className="h-6 flex-shrink-0 text-text-primary text-base">
              {data?.source ? DataSourceInfo[data?.source].name : ''}
            </div> */}
            {data?.name}
          </div>
        </div>
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
    titleClassName: 'border-b border-border-button',
    onVisibleChange: () => {
      Modal.destroy();
    },
    footer: (
      <div className="flex justify-end gap-2">
        <Button variant={'outline'} onClick={() => Modal.destroy()}>
          {t('dataflowParser.changeStepModalCancelText')}
        </Button>
        <Button
          variant={'secondary'}
          className="!bg-state-error text-text-base"
          onClick={() => {
            onOk?.(data);
            Modal.destroy();
          }}
        >
          {config.confirmText}
        </Button>
      </div>
    ),
  });
};
