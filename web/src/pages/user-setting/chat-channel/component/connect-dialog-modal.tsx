/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
