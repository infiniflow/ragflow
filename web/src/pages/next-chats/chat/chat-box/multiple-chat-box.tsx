import { LargeModelFormFieldWithoutFilter } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/message-item';
import PdfDrawer from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { MessageType } from '@/constants/chat';
import { useScrollToBottom } from '@/hooks/logic-hooks';
import {
  useFetchConversation,
  useFetchDialog,
  useGetChatSearchParams,
  useSetDialog,
} from '@/hooks/use-chat-request';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty, omit } from 'lodash';
import { ListCheck, Plus, Trash2 } from 'lucide-react';
import { forwardRef, useCallback, useImperativeHandle, useRef } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useParams } from 'umi';
import { z } from 'zod';
import {
  useGetSendButtonDisabled,
  useSendButtonDisabled,
} from '../../hooks/use-button-disabled';
import { useCreateConversationBeforeUploadDocument } from '../../hooks/use-create-conversation';
import { useSendMessage } from '../../hooks/use-send-chat-message';
import { useSendMultipleChatMessage } from '../../hooks/use-send-multiple-message';
import { buildMessageItemReference } from '../../utils';
import { IMessage } from '../interface';
import { useAddChatBox } from '../use-add-box';

type MultipleChatBoxProps = {
  controller: AbortController;
  chatBoxIds: string[];
} & Pick<
  ReturnType<typeof useAddChatBox>,
  'removeChatBox' | 'addChatBox' | 'chatBoxIds'
>;

type ChatCardProps = {
  id: string;
  idx: number;
  derivedMessages: IMessage[];
  sendLoading: boolean;
} & Pick<
  MultipleChatBoxProps,
  'controller' | 'removeChatBox' | 'addChatBox' | 'chatBoxIds'
> &
  Pick<ReturnType<typeof useClickDrawer>, 'clickDocumentButton'>;

const ChatCard = forwardRef(function ChatCard(
  {
    controller,
    removeChatBox,
    id,
    idx,
    addChatBox,
    chatBoxIds,
    derivedMessages,
    sendLoading,
    clickDocumentButton,
  }: ChatCardProps,
  ref,
) {
  const { id: dialogId } = useParams();
  const { setDialog } = useSetDialog();

  const { regenerateMessage, removeMessageById } = useSendMessage(controller);

  const messageContainerRef = useRef<HTMLDivElement>(null);

  const { scrollRef } = useScrollToBottom(derivedMessages, messageContainerRef);

  const FormSchema = z.object(LlmSettingSchema);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      llm_id: '',
    },
  });

  const llmId = useWatch({ control: form.control, name: 'llm_id' });

  const { data: userInfo } = useFetchUserInfo();
  const { data: currentDialog } = useFetchDialog();
  const { data: conversation } = useFetchConversation();

  const isLatestChat = idx === chatBoxIds.length - 1;

  const handleRemoveChatBox = useCallback(() => {
    removeChatBox(id);
  }, [id, removeChatBox]);

  const handleApplyConfig = useCallback(() => {
    const values = form.getValues();
    setDialog({
      ...currentDialog,
      llm_id: values.llm_id,
      llm_setting: omit(values, 'llm_id'),
      dialog_id: dialogId,
    });
  }, [currentDialog, dialogId, form, setDialog]);

  useImperativeHandle(ref, () => ({
    getFormData: () => form.getValues(),
  }));

  return (
    <Card className="bg-transparent border flex-1 flex flex-col">
      <CardHeader className="border-b px-5 py-3">
        <CardTitle className="flex justify-between items-center">
          <div className="flex items-center gap-3">
            <span className="text-base">{idx + 1}</span>
            <Form {...form}>
              <LargeModelFormFieldWithoutFilter></LargeModelFormFieldWithoutFilter>
            </Form>
          </div>
          <div className="space-x-2">
            <Tooltip>
              <TooltipTrigger>
                <Button
                  variant={'ghost'}
                  disabled={isEmpty(llmId)}
                  onClick={handleApplyConfig}
                >
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
      <CardContent className="flex-1 min-h-0">
        <div ref={messageContainerRef} className="h-full overflow-auto">
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
                  clickDocumentButton={clickDocumentButton}
                ></MessageItem>
              );
            })}
          </div>
          <div ref={scrollRef} />
        </div>
      </CardContent>
    </Card>
  );
});

export function MultipleChatBox({
  controller,
  chatBoxIds,
  removeChatBox,
  addChatBox,
}: MultipleChatBoxProps) {
  const {
    value,
    sendLoading,
    messageRecord,
    handleInputChange,
    handlePressEnter,
    stopOutputMessage,
    setFormRef,
    handleUploadFile,
  } = useSendMultipleChatMessage(controller, chatBoxIds);

  const { createConversationBeforeUploadDocument } =
    useCreateConversationBeforeUploadDocument();
  const { conversationId } = useGetChatSearchParams();
  const disabled = useGetSendButtonDisabled();
  const sendDisabled = useSendButtonDisabled(value);
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  return (
    <section className="h-full flex flex-col px-5">
      <div className="flex gap-4 flex-1 px-5 pb-14 min-h-0">
        {chatBoxIds.map((id, idx) => (
          <ChatCard
            key={id}
            idx={idx}
            controller={controller}
            id={id}
            chatBoxIds={chatBoxIds}
            removeChatBox={removeChatBox}
            addChatBox={addChatBox}
            derivedMessages={messageRecord[id]}
            ref={setFormRef(id)}
            sendLoading={sendLoading}
            clickDocumentButton={clickDocumentButton}
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
          onUpload={handleUploadFile}
        />
      </div>
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
}
