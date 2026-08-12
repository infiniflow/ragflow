import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/message-item';
import PdfSheet from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { MessageType } from '@/constants/chat';
import { useFetchChat, useGetChatSearchParams } from '@/hooks/use-chat-request';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { IClientConversation } from '@/interfaces/database/chat';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { useEffect } from 'react';
import {
  useGetSendButtonDisabled,
  useSendButtonDisabled,
} from '../../hooks/use-button-disabled';
import { useCreateConversationBeforeUploadDocument } from '../../hooks/use-create-conversation';
import { useSendMessage } from '../../hooks/use-send-chat-message';
import { buildMessageItemReference } from '../../utils';
import { useShowInternet } from '../use-show-internet';

interface IProps {
  controller: AbortController;
  stopOutputMessage(): void;
  conversation: IClientConversation;
}

export function SingleChatBox({
  controller,
  stopOutputMessage,
  conversation,
}: IProps) {
  const {
    value,
    scrollRef,
    messageContainerRef,
    sendLoading,
    derivedMessages,
    isUploading,
    handleInputChange,
    handlePressEnter,
    regenerateMessage,
    removeMessageById,
    handleUploadFile,
    removeFile,
    setDerivedMessages,
    activeStreamsRef,
  } = useSendMessage(controller);
  const { data: userInfo } = useFetchUserInfo();
  const { data: currentDialog } = useFetchChat();
  const { createConversationBeforeUploadDocument } =
    useCreateConversationBeforeUploadDocument();
  const { conversationId } = useGetChatSearchParams();
  const disabled = useGetSendButtonDisabled();
  const sendDisabled = useSendButtonDisabled(value);
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  const showInternet = useShowInternet();

  useEffect(() => {
    // Don't let backend data overwrite local streaming state while SSE is
    // in-flight for this conversation. The server hasn't persisted the latest
    // answer yet, and applying conversation.messages would discard the
    // in-progress answer.
    if (activeStreamsRef.current.has(conversationId)) return;

    // Skip when the conversation prop is stale — its id doesn't match the
    // URL's current conversationId. This happens during a switch (e.g.
    // clicking "+" to create a new session): child effects fire before the
    // parent's clear/load effect, so for one render the prop still holds the
    // previous conversation's messages. Applying them here would leak the old
    // conversation's content into the newly switched (or new) conversation.
    // The cache + prologue logic in useSelectNextMessages handles restoring
    // or seeding messages for the new conversationId.
    if (conversation?.id && conversation.id !== conversationId) return;

    const messages = conversation?.messages;
    if (Array.isArray(messages)) {
      setDerivedMessages((prevMessages) => {
        // Preserve uploaded file objects from local state that the server doesn't
        // persist (e.g. File instances). Build a map of message id → files from
        // the current local state so they survive when server data is applied.
        const filesMap = new Map(
          prevMessages
            .filter((m) => m.files?.length)
            .map((m) => [m.id, m.files]),
        );
        if (filesMap.size === 0) {
          return messages;
        }
        return messages.map((m) => ({
          ...m,
          files: filesMap.get(m.id) ?? m.files,
        }));
      });
    }
  }, [
    conversation?.messages,
    conversation?.id,
    conversationId,
    setDerivedMessages,
    activeStreamsRef,
  ]);

  useEffect(() => {
    // Clear the message list after deleting the conversation.
    if (conversationId === '') {
      setDerivedMessages([]);
    }
  }, [conversationId, setDerivedMessages]);

  return (
    <section className="flex flex-col h-full gap-4">
      <div
        ref={messageContainerRef}
        className="p-5 flex-1 overflow-auto min-h-0 scrollbar-auto"
      >
        <div className="w-full pr-5">
          {derivedMessages?.map((message, i) => (
            <MessageItem
              loading={
                message.role === MessageType.Assistant &&
                sendLoading &&
                derivedMessages.length - 1 === i
              }
              key={buildMessageUuidWithRole(message)}
              item={message}
              nickname={userInfo.nickname}
              avatar={userInfo.avatar}
              avatarDialog={currentDialog.icon}
              reference={buildMessageItemReference(
                {
                  messages: derivedMessages,
                  reference: conversation.reference,
                },
                message,
              )}
              clickDocumentButton={clickDocumentButton}
              index={i}
              removeMessageById={removeMessageById}
              regenerateMessage={regenerateMessage}
              sendLoading={sendLoading}
            />
          ))}
        </div>
        <div ref={scrollRef} />
      </div>

      <div className="p-5 pt-0">
        <NextMessageInput
          disabled={disabled}
          sendDisabled={sendDisabled}
          sendLoading={sendLoading}
          value={value}
          resize="vertical"
          onInputChange={handleInputChange}
          onPressEnter={handlePressEnter}
          conversationId={conversationId}
          createConversationBeforeUploadDocument={
            createConversationBeforeUploadDocument
          }
          stopOutputMessage={stopOutputMessage}
          onUpload={handleUploadFile}
          isUploading={isUploading}
          removeFile={removeFile}
          showReasoning
          showInternet={showInternet}
        />
        {visible && (
          <PdfSheet
            visible={visible}
            hideModal={hideModal}
            documentId={documentId}
            chunk={selectedChunk}
          ></PdfSheet>
        )}
      </div>
    </section>
  );
}
