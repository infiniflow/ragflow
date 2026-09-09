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
import { useFetchAllAgentList } from '@/hooks/use-agent-request';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useChatChannelDialogList,
  useConnectChatChannelDialog,
} from '../hooks';
import { IChatChannelBase } from '../interface';
import { toChatChannelTargetValue } from '../connect-target';

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
  const { data: agents } = useFetchAllAgentList();
  const { connect, connecting } = useConnectChatChannelDialog();
  const [targetValue, setTargetValue] = useState<string | undefined>(
    channel?.agent_id
      ? toChatChannelTargetValue('agent', channel.agent_id)
      : toChatChannelTargetValue('chat', channel?.chat_id),
  );

  useEffect(() => {
    setTargetValue(
      channel?.agent_id
        ? toChatChannelTargetValue('agent', channel.agent_id)
        : toChatChannelTargetValue('chat', channel?.chat_id),
    );
  }, [channel?.id, channel?.chat_id, channel?.agent_id]);

  const options = useMemo(
    () => [
      ...(dialogs || []).map((d) => ({
        label: `[${t('setting.chatChannelAssistant')}] ${d.name}`,
        value: toChatChannelTargetValue('chat', d.id)!,
      })),
      ...(agents || [])
        .filter((agent) => 'title' in agent)
        .map((agent) => ({
          label: `[${t('setting.chatChannelAgent')}] ${agent.title}`,
          value: toChatChannelTargetValue('agent', agent.id)!,
        })),
    ],
    [agents, dialogs, t],
  );

  const handleConfirm = async () => {
    if (!channel) {
      return;
    }
    await connect({ channelId: channel.id, targetValue });
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
          value={targetValue}
          onChange={(val: string) => setTargetValue(val || undefined)}
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
