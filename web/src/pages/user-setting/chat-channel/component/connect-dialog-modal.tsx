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
  useChatChannelAgentList,
  useChatChannelDialogList,
  useConnectChatChannelTarget,
} from '../hooks';
import { IChatChannelBase } from '../interface';

type TargetType = 'dialog' | 'agent';

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
  const { agents } = useChatChannelAgentList();
  const { connect, connecting } = useConnectChatChannelTarget();
  const [targetType, setTargetType] = useState<TargetType>('dialog');
  const [dialogId, setDialogId] = useState<string | undefined>(
    channel?.chat_id ?? undefined,
  );
  const [agentId, setAgentId] = useState<string | undefined>(
    channel?.agent_id ?? undefined,
  );

  useEffect(() => {
    setDialogId(channel?.chat_id ?? undefined);
    setAgentId(channel?.agent_id ?? undefined);
    setTargetType(channel?.agent_id ? 'agent' : 'dialog');
  }, [channel?.id, channel?.chat_id, channel?.agent_id]);

  const dialogOptions = useMemo(
    () => (dialogs || []).map((d) => ({ label: d.name, value: d.id })),
    [dialogs],
  );
  const agentOptions = useMemo(
    () =>
      (agents || []).map((a) => ({
        label: 'title' in a ? a.title : a.id,
        value: a.id,
      })),
    [agents],
  );

  const handleConfirm = async () => {
    if (!channel) {
      return;
    }
    await connect({
      channelId: channel.id,
      dialogId: targetType === 'dialog' ? dialogId || null : null,
      agentId: targetType === 'agent' ? agentId || null : null,
    });
    hideModal();
  };

  const targetTypeButtonClass = (active: boolean) =>
    `flex-1 px-2 py-1.5 text-sm rounded-md border transition-colors ${
      active
        ? 'bg-bg-card text-text-primary border-border-button'
        : 'text-text-secondary border-transparent hover:bg-bg-card'
    }`;

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
          {t('setting.connectTargetType')}
        </label>
        <div className="flex gap-2 bg-bg-card rounded-md p-1">
          <button
            type="button"
            className={targetTypeButtonClass(targetType === 'dialog')}
            onClick={() => setTargetType('dialog')}
          >
            {t('setting.targetAssistant')}
          </button>
          <button
            type="button"
            className={targetTypeButtonClass(targetType === 'agent')}
            onClick={() => setTargetType('agent')}
          >
            {t('setting.targetAgent')}
          </button>
        </div>
        {targetType === 'dialog' ? (
          <>
            <label className="text-sm text-text-secondary">
              {t('setting.selectDialog')}
            </label>
            <RAGFlowSelect
              value={dialogId}
              onChange={(val: string) => setDialogId(val || undefined)}
              options={dialogOptions}
              allowClear
              placeholder={t('setting.selectDialog')}
            />
            <p className="text-xs text-text-secondary/70 mt-1">
              {t('setting.connectDialogTip')}
            </p>
          </>
        ) : (
          <>
            <label className="text-sm text-text-secondary">
              {t('setting.selectAgent')}
            </label>
            <RAGFlowSelect
              value={agentId}
              onChange={(val: string) => setAgentId(val || undefined)}
              options={agentOptions}
              allowClear
              placeholder={t('setting.selectAgent')}
            />
            <p className="text-xs text-text-secondary/70 mt-1">
              {t('setting.connectAgentTip')}
            </p>
          </>
        )}
      </div>
    </Modal>
  );
};

export default ConnectDialogModal;
