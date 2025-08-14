import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/message-item';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { MessageType } from '@/constants/chat';
import {
  useFetchConversation,
  useFetchDialog,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { ListCheck, Plus, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import {
  useGetSendButtonDisabled,
  useSendButtonDisabled,
} from '../../hooks/use-button-disabled';
import { useCreateConversationBeforeUploadDocument } from '../../hooks/use-create-conversation';
import { useSendMessage } from '../../hooks/use-send-chat-message';
import { buildMessageItemReference } from '../../utils';
import { LLMSelectForm } from '../llm-select-form';
import { useAddChatBox } from '../use-add-box';

type MultipleChatBoxProps = {
  controller: AbortController;
  chatBoxIds: string[];
} & Pick<
  ReturnType<typeof useAddChatBox>,
  'removeChatBox' | 'addChatBox' | 'chatBoxIds'
>;

type ChatCardProps = { id: string; idx: number } & Pick<
  MultipleChatBoxProps,
  'controller' | 'removeChatBox' | 'addChatBox' | 'chatBoxIds'
>;

function ChatCard({
  controller,
  removeChatBox,
  id,
  idx,
  addChatBox,
  chatBoxIds,
}: ChatCardProps) {
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

  const isLatestChat = idx === chatBoxIds.length - 1;

  const handleRemoveChatBox = useCallback(() => {
    removeChatBox(id);
  }, [id, removeChatBox]);

  return (
    <Card className="bg-transparent border flex-1">
      <CardHeader className="border-b px-5 py-3">
        <CardTitle className="flex justify-between items-center">
          <div className="flex items-center gap-3">
            <span className="text-base">{idx + 1}</span>
            <LLMSelectForm></LLMSelectForm>
          </div>
          <div className="space-x-2">
            <Tooltip>
              <TooltipTrigger>
                <Button variant={'ghost'}>
                  <ListCheck />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Apply model configs</p>
              </TooltipContent>
            </Tooltip>
            {!isLatestChat || chatBoxIds.length === 3 ? (
              <Button variant={'ghost'} onClick={handleRemoveChatBox}>
                <Trash2 />
              </Button>
            ) : (
              <Button variant={'ghost'} onClick={addChatBox}>
                <Plus></Plus>
              </Button>
            )}
          </div>
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
  addChatBox,
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
    <section className="h-full flex flex-col px-5">
      <div className="flex gap-4 flex-1 px-5 pb-14">
        {chatBoxIds.map((id, idx) => (
          <ChatCard
            key={id}
            idx={idx}
            controller={controller}
            id={id}
            chatBoxIds={chatBoxIds}
            removeChatBox={removeChatBox}
            addChatBox={addChatBox}
          ></ChatCard>
        ))}
      </div>
      <div className="px-[20%]">
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
      </div>
    </section>
  );
}
