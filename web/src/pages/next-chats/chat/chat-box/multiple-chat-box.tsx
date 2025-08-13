import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/message-item';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { MessageType } from '@/constants/chat';
import {
  useFetchConversation,
  useFetchDialog,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import {
  useGetSendButtonDisabled,
  useSendButtonDisabled,
} from '../../hooks/use-button-disabled';
import { useCreateConversationBeforeUploadDocument } from '../../hooks/use-create-conversation';
import { useSendMessage } from '../../hooks/use-send-chat-message';
import { buildMessageItemReference } from '../../utils';
import { useAddChatBox } from '../use-add-box';

type MultipleChatBoxProps = {
  controller: AbortController;
  chatBoxIds: string[];
} & Pick<ReturnType<typeof useAddChatBox>, 'removeChatBox'>;

type ChatCardProps = { id: string } & Pick<
  MultipleChatBoxProps,
  'controller' | 'removeChatBox'
>;

function ChatCard({ controller, removeChatBox, id }: ChatCardProps) {
  const {
    value,
    // scrollRef,
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
  const { data: conversation } = useFetchConversation();

  const handleRemoveChatBox = useCallback(() => {
    removeChatBox(id);
  }, [id, removeChatBox]);

  return (
    <Card className="bg-transparent border flex-1">
      <CardHeader className="border-b px-5 py-3">
        <CardTitle className="flex justify-between items-center">
          <div>
            <span className="text-base">Card Title</span>
            <Button variant={'ghost'} className="ml-2">
              GPT-4
            </Button>
          </div>
          <Button variant={'ghost'} onClick={handleRemoveChatBox}>
            <Trash2 />
          </Button>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div ref={messageContainerRef} className="flex-1 overflow-auto min-h-0">
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
          {/* <div ref={scrollRef} /> */}
        </div>
      </CardContent>
    </Card>
  );
}

export function MultipleChatBox({
  controller,
  chatBoxIds,
  removeChatBox,
}: MultipleChatBoxProps) {
  const {
    value,
    sendLoading,
    handleInputChange,
    handlePressEnter,
    stopOutputMessage,
  } = useSendMessage(controller);

  const { createConversationBeforeUploadDocument } =
    useCreateConversationBeforeUploadDocument();
  const { conversationId } = useGetChatSearchParams();
  const disabled = useGetSendButtonDisabled();
  const sendDisabled = useSendButtonDisabled(value);
  return (
    <section className="h-full flex flex-col">
      <div className="flex gap-4 flex-1 px-5 pb-12">
        {chatBoxIds.map((id) => (
          <ChatCard
            key={id}
            controller={controller}
            id={id}
            removeChatBox={removeChatBox}
          ></ChatCard>
        ))}
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
