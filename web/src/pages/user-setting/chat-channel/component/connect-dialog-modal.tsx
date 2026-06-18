import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { RAGFlowSelect } from '@/components/ui/select';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useChatChannelDialogList,
  useConnectChatChannelDialog,
} from '../hooks';
import { IChatChannelBase } from '../interface';

const ConnectDialogModal = ({
  visible,
  hideModal,
  channel,
}: {
  visible: boolean;
  hideModal: () => void;
  channel?: IChatChannelBase;
}) => {
  const { t } = useTranslation();
  const { dialogs } = useChatChannelDialogList();
  const { connect, connecting } = useConnectChatChannelDialog();
  const [dialogId, setDialogId] = useState<string | undefined>(
    channel?.chat_id ?? undefined,
  );

  useEffect(() => {
    setDialogId(channel?.chat_id ?? undefined);
  }, [channel?.id, channel?.chat_id]);

  const options = useMemo(
    () => (dialogs || []).map((d) => ({ label: d.name, value: d.id })),
    [dialogs],
  );

  const handleConfirm = async () => {
    if (!channel) {
      return;
    }
    await connect({ channelId: channel.id, dialogId: dialogId || null });
    hideModal();
  };

  return (
    <Modal
      title={t('setting.connectDialogTitle', { name: channel?.name })}
      open={visible}
      maskClosable={false}
      onOpenChange={(open) => !open && hideModal()}
      footer={
        <div className="flex justify-end gap-2">
          <Button variant={'outline'} onClick={hideModal}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={connecting}>
            {t('common.confirm')}
          </Button>
        </div>
      }
    >
      <div className="px-2 py-4 flex flex-col gap-1.5">
        <label className="text-sm text-text-secondary">
          {t('setting.selectDialog')}
        </label>
        <RAGFlowSelect
          value={dialogId}
          onChange={(val: string) => setDialogId(val || undefined)}
          options={options}
          allowClear
          placeholder={t('setting.selectDialog')}
        />
        <p className="text-xs text-text-secondary/70 mt-1">
          {t('setting.connectDialogTip')}
        </p>
      </div>
    </Modal>
  );
};

export default ConnectDialogModal;
