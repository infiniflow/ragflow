import MessageItem from '@/components/next-message-item';
import { Modal } from '@/components/ui/modal/modal';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { IAgentLogMessage } from '@/interfaces/database/agent';
import { IReferenceObject, Message } from '@/interfaces/database/chat';
import { buildMessageUuidWithRole } from '@/utils/chat';
import React, { useMemo } from 'react';
import { IMessage } from '../chat/interface';

interface CustomModalProps {
  isOpen: boolean;
  onClose: () => void;
  message: IAgentLogMessage[];
  reference: IReferenceObject;
}

export const AgentLogDetailModal: React.FC<CustomModalProps> = ({
  isOpen,
  onClose,
  message: derivedMessages,
  reference,
}) => {
  const { data: userInfo } = useFetchUserInfo();
  const { data: canvasInfo } = useFetchAgent();

  const shortMessage = useMemo(() => {
    if (derivedMessages?.length) {
      const content = derivedMessages[0]?.content || '';

      const chineseCharCount = (content.match(/[\u4e00-\u9fa5]/g) || []).length;
      const totalLength = content.length;

      if (chineseCharCount > 0) {
        if (totalLength > 15) {
          return content.substring(0, 15) + '...';
        }
      } else {
        if (totalLength > 30) {
          return content.substring(0, 30) + '...';
        }
      }
      return content;
    } else {
      return '';
    }
  }, [derivedMessages]);

  return (
    <Modal
      open={isOpen}
      onCancel={onClose}
      showfooter={false}
      footer={null}
      title={shortMessage || ''}
      className="!w-[900px]"
    >
      <div className="flex items-start mb-4 flex-col gap-4 justify-start">
        <div className="w-full">
          {derivedMessages?.map((message, i) => {
            return (
              <MessageItem
                key={buildMessageUuidWithRole(
                  message as Partial<Message | IMessage>,
                )}
                nickname={userInfo.nickname}
                avatar={userInfo.avatar}
                avatarDialog={canvasInfo.avatar}
                item={message as IMessage}
                reference={reference}
                index={i}
                showLikeButton={false}
                showLog={false}
              ></MessageItem>
            );
          })}
        </div>
      </div>
    </Modal>
  );
};
