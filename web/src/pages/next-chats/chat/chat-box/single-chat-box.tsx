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
  }, [conversation?.messages, setDerivedMessages]);

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
          {/*
            业务规则：用户按发送键后，derivedMessages 在 addNewestQuestion 阶段已经把
            assistant 占位一起追加进去了（content=''），理想情况下 MessageItem
            的 showWaitingForResponse 条件会立即满足并渲染"AI 正在思考"气泡。
            但当前端在弱网、慢模型或 SSE 第一个事件延迟（>1s）时，从用户按下
            到看到首字的体感仍然偏长。这里加一道显式兜底：sendLoading=true
            且消息列表末位不是 assistant 时，直接渲染一个乐观占位气泡，
            确保按下到反馈的间隔 < 16ms（一帧）。
          */}
          {sendLoading &&
            (!derivedMessages?.length ||
              derivedMessages[derivedMessages.length - 1].role !==
                MessageType.Assistant) && (
              <MessageItem
                loading={true}
                key="__optimistic_assistant_placeholder__"
                item={{
                  id: '__optimistic_assistant_placeholder__',
                  role: MessageType.Assistant,
                  content: '',
                  conversationId: conversationId ?? '',
                }}
                nickname={userInfo.nickname}
                avatar={userInfo.avatar}
                avatarDialog={currentDialog.icon}
                reference={{ chunks: [], doc_aggs: [], total: 0 }}
                clickDocumentButton={clickDocumentButton}
                index={derivedMessages?.length ?? 0}
                removeMessageById={removeMessageById}
                regenerateMessage={regenerateMessage}
                sendLoading={true}
              />
            )}
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
