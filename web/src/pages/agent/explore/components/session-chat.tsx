import MessageItem from '@/components/next-message-item';
import { MessageType } from '@/constants/chat';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import {
  IAgentLogMessage,
  IAgentLogResponse,
} from '@/interfaces/database/agent';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface SessionChatProps {
  canvasId: string;
  sessionId?: string;
}

export function SessionChat({ canvasId, sessionId }: SessionChatProps) {
  const { t } = useTranslation();
  const { data: userInfo } = useFetchUserInfo();
  const [sessionData, setSessionData] = useState<IAgentLogResponse | null>(
    null,
  );
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!sessionId) {
      setSessionData(null);
      return;
    }

    const fetchSessionData = async () => {
      setLoading(true);
      try {
        const { fetchAgentLogsByCanvasId } =
          await import('@/services/agent-service');
        const { data } = await fetchAgentLogsByCanvasId(canvasId, {
          page: 1,
          page_size: 100,
        });
        const sessions = data?.data?.sessions ?? [];
        const foundSession = sessions.find(
          (s: IAgentLogResponse) => s.id === sessionId,
        );
        setSessionData(foundSession || null);
      } catch (error) {
        console.error('Failed to fetch session:', error);
        setSessionData(null);
      } finally {
        setLoading(false);
      }
    };

    fetchSessionData();
  }, [canvasId, sessionId]);

  const messages =
    sessionData?.message?.map((msg: IAgentLogMessage) => ({
      role: msg.role === 'assistant' ? MessageType.Assistant : MessageType.User,
      content: msg.content,
      id: buildMessageUuidWithRole({ role: msg.role, id: msg.id }),
      ...(msg.role === 'assistant' && {
        reference: sessionData.reference,
      }),
    })) ?? [];

  return (
    <section className="flex flex-col h-full">
      {!sessionId && (
        <div className="flex-1 flex items-center justify-center text-text-secondary">
          {t('explore.noSessionSelected')}
        </div>
      )}

      {sessionId && (
        <div className="flex-1 overflow-auto p-5">
          {loading ? (
            <div className="flex items-center justify-center h-full">
              Loading...
            </div>
          ) : messages.length === 0 ? (
            <div className="flex items-center justify-center h-full text-text-secondary">
              No messages in this session
            </div>
          ) : (
            <div className="w-full">
              {messages.map((message, i) => (
                <MessageItem
                  key={message.id}
                  item={message}
                  nickname={userInfo.nickname}
                  avatar={userInfo.avatar}
                  avatarDialog={''} // TODO: get canvas avatar
                  reference={message.reference}
                  clickDocumentButton={() => {}}
                  index={i}
                  showLikeButton={false}
                  sendLoading={false}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}
