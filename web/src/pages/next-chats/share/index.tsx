import { FileUploadProps } from '@/components/file-upload';
import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/next-message-item';
import PdfDrawer from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { MessageType, SharedFrom } from '@/constants/chat';
import { useFetchNextConversationSSE } from '@/hooks/chat-hooks';
import {
  useFetchAgentAvatar,
  useUploadCanvasFileWithProgress,
} from '@/hooks/use-agent-request';
import { cn } from '@/lib/utils';
import i18n from '@/locales/config';
import { useCacheChatLog } from '@/pages/agent/hooks/use-cache-chat-log';
import { useSendButtonDisabled } from '@/pages/chat/hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import React, { forwardRef, useCallback, useMemo } from 'react';
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

  const { uploadCanvasFile, loading } =
    useUploadCanvasFileWithProgress(conversationId);
  const {
    addEventList,
    setCurrentMessageId,
    currentEventListWithoutMessageById,
    clearEventList,
    currentMessageId,
  } = useCacheChatLog();
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
    appendUploadResponseList,
  } = useSendNextSharedMessage(addEventList);

  const sendDisabled = useSendButtonDisabled(value);

  // useEffect(() => {
  //   if (derivedMessages.length) {
  //     const derivedMessagesFilter = derivedMessages.filter(
  //       (message) => message.role === MessageType.Assistant,
  //     );
  //     if (derivedMessagesFilter.length) {
  //       const message = derivedMessagesFilter[derivedMessagesFilter.length - 1];
  //       setCurrentMessageId(message.id);
  //     }
  //   }
  // }, [derivedMessages, setCurrentMessageId]);

  const useFetchAvatar = useMemo(() => {
    return from === SharedFrom.Agent
      ? useFetchAgentAvatar
      : useFetchNextConversationSSE;
  }, [from]);

  const handleUploadFile: NonNullable<FileUploadProps['onUpload']> =
    useCallback(
      async (files, options) => {
        const ret = await uploadCanvasFile({ files, options });
        appendUploadResponseList(ret.data);
      },
      [appendUploadResponseList, uploadCanvasFile],
    );

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
                  conversationId={conversationId}
                  currentEventListWithoutMessageById={
                    currentEventListWithoutMessageById
                  }
                  setCurrentMessageId={setCurrentMessageId}
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
                  sendLoading={sendLoading}
                ></MessageItem>
              );
            })}
          </div>
          <div ref={ref} />
        </div>
        <NextMessageInput
          isShared
          value={value}
          disabled={hasError}
          sendDisabled={sendDisabled}
          conversationId={conversationId}
          onInputChange={handleInputChange}
          onPressEnter={handlePressEnter}
          sendLoading={sendLoading}
          stopOutputMessage={stopOutputMessage}
          onUpload={handleUploadFile}
          isUploading={loading}
        ></NextMessageInput>
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
