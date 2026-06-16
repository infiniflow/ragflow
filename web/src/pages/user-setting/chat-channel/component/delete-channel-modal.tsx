import { Button } from '@/components/ui/button';
import { Modal, ModalType } from '@/components/ui/modal/modal';
import { t } from 'i18next';
import { IChatChannelBase, IChatChannelInfoMap } from '../interface';

export type IDelChannelModalProps = Partial<ModalType> & {
  data?: IChatChannelBase;
  onOk?: (data?: IChatChannelBase) => void;
  chatChannelInfo: IChatChannelInfoMap;
};

export const delChannelModal = (props: IDelChannelModalProps) => {
  const { data, onOk, chatChannelInfo, ...otherProps } = props;
  Modal.show({
    visible: true,
    className: '!w-[560px]',
    ...otherProps,
    title: t('setting.deleteChannelModalTitle'),
    titleClassName: 'border-b border-border-button',
    children: (
      <div className="px-2 space-y-6 pt-5 pb-3">
        <div
          className="text-base text-text-primary"
          dangerouslySetInnerHTML={{
            __html: t('setting.deleteChannelModalContent'),
          }}
        />
        <div className="flex items-center gap-1 p-2 border border-border-button rounded-md mb-3">
          <div className="w-6 h-6 flex-shrink-0">
            {data?.channel ? chatChannelInfo[data.channel].icon : ''}
          </div>
          <div className="flex items-center gap-2 text-text-secondary text-xs">
            {data?.name}
          </div>
        </div>
      </div>
    ),
    onVisibleChange: () => {
      Modal.destroy();
    },
    footer: (
      <div className="flex justify-end gap-2">
        <Button variant={'outline'} onClick={() => Modal.destroy()}>
          {t('common.cancel')}
        </Button>
        <Button
          variant={'secondary'}
          className="!bg-state-error text-text-base"
          onClick={() => {
            onOk?.(data);
            Modal.destroy();
          }}
        >
          {t('common.delete')}
        </Button>
      </div>
    ),
  });
};
