import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/message-item';
import { MessageType } from '@/constants/chat';
import {
  useFetchConversation,
  useFetchDialog,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import {
  useGetSendButtonDisabled,
  useSendButtonDisabled,
} from '../hooks/use-button-disabled';
import { useCreateConversationBeforeUploadDocument } from '../hooks/use-create-conversation';
import { useSendMessage } from '../hooks/use-send-chat-message';
import { buildMessageItemReference } from '../utils';

interface IProps {
  controller: AbortController;
}

export function ChatBox({ controller }: IProps) {
  const {
    value,
    scrollRef,
    messageContainerRef,
    sendLoading,
    derivedMessages,
    handleInputChange,
    handlePressEnter,
    regenerateMessage,
    removeMessageById,
    stopOutputMessage,
  } = useSendMessage(controller);
  const { data: userInfo } = useFetchUserInfo();
  const { data: currentDialog } = useFetchDialog();
  const { createConversationBeforeUploadDocument } =
    useCreateConversationBeforeUploadDocument();
  const { conversationId } = useGetChatSearchParams();
  const { data: conversation } = useFetchConversation();
  const disabled = useGetSendButtonDisabled();
  const sendDisabled = useSendButtonDisabled(value);

  return (
    <section className="border-x  flex flex-col p-5 w-full">
      <div ref={messageContainerRef} className="flex-1 overflow-auto">
        <div className="w-full">
          {derivedMessages?.map((message, i) => {
            return (
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
                    message: derivedMessages,
                    reference: conversation.reference,
                  },
                  message,
                )}
                // clickDocumentButton={clickDocumentButton}
                index={i}
                removeMessageById={removeMessageById}
                regenerateMessage={regenerateMessage}
                sendLoading={sendLoading}
              ></MessageItem>
            );
          })}
        </div>
        <div ref={scrollRef} />
      </div>
      <NextMessageInput
        disabled={disabled}
        sendDisabled={sendDisabled}
        sendLoading={sendLoading}
        value={value}
        onInputChange={handleInputChange}
        onPressEnter={handlePressEnter}
        conversationId={conversationId}
        createConversationBeforeUploadDocument={
          createConversationBeforeUploadDocument
        }
        stopOutputMessage={stopOutputMessage}
      />
    </section>
  );
}
