import MessageInput from '@/components/message-input';
import MessageItem from '@/components/next-message-item';
import PdfDrawer from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { MessageType, SharedFrom } from '@/constants/chat';
import { useFetchNextConversationSSE } from '@/hooks/chat-hooks';
import { useFetchAgentAvatar } from '@/hooks/use-agent-request';
import { cn } from '@/lib/utils';
import i18n from '@/locales/config';
import { useSendButtonDisabled } from '@/pages/chat/hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import React, { forwardRef, useMemo } from 'react';
import {
  useGetSharedChatSearchParams,
  useSendNextSharedMessage,
} from '../hooks/use-send-shared-message';

const ChatContainer = () => {
  const {
    sharedId: conversationId,
    from,
    locale,
    visibleAvatar,
  } = useGetSharedChatSearchParams();
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  const {
    handlePressEnter,
    handleInputChange,
    value,
    sendLoading,
    ref,
    derivedMessages,
    hasError,
    stopOutputMessage,
    findReferenceByMessageId,
  } = useSendNextSharedMessage();
  const sendDisabled = useSendButtonDisabled(value);

  const useFetchAvatar = useMemo(() => {
    return from === SharedFrom.Agent
      ? useFetchAgentAvatar
      : useFetchNextConversationSSE;
  }, [from]);

  React.useEffect(() => {
    if (locale && i18n.language !== locale) {
      i18n.changeLanguage(locale);
    }
  }, [locale, visibleAvatar]);
  const { data: avatarData } = useFetchAvatar();

  if (!conversationId) {
    return <div>empty</div>;
  }

  return (
    <section className="h-[100vh]">
      <section className={cn('flex flex-1 flex-col p-2.5 h-full')}>
        <div className={cn('flex flex-1 flex-col overflow-auto pr-2')}>
          <div>
            {derivedMessages?.map((message, i) => {
              return (
                <MessageItem
                  visibleAvatar={visibleAvatar}
                  key={buildMessageUuidWithRole(message)}
                  avatarDialog={avatarData.avatar}
                  item={message}
                  nickname="You"
                  reference={findReferenceByMessageId(message.id)}
                  loading={
                    message.role === MessageType.Assistant &&
                    sendLoading &&
                    derivedMessages?.length - 1 === i
                  }
                  index={i}
                  clickDocumentButton={clickDocumentButton}
                  showLikeButton={false}
                  showLoudspeaker={false}
                  showLog={false}
                ></MessageItem>
              );
            })}
          </div>
          <div ref={ref} />
        </div>

        <MessageInput
          isShared
          value={value}
          disabled={hasError}
          sendDisabled={sendDisabled}
          conversationId={conversationId}
          onInputChange={handleInputChange}
          onPressEnter={handlePressEnter}
          sendLoading={sendLoading}
          uploadMethod="external_upload_and_parse"
          showUploadIcon={false}
          stopOutputMessage={stopOutputMessage}
        ></MessageInput>
      </section>
      {visible && (
        <PdfDrawer
          visible={visible}
          hideModal={hideModal}
          documentId={documentId}
          chunk={selectedChunk}
        ></PdfDrawer>
      )}
    </section>
  );
};

export default forwardRef(ChatContainer);
