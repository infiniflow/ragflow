import { LargeModelFormFieldWithoutFilter } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import {
  NextMessageInput,
  NextMessageInputOnPressEnterParameter,
} from '@/components/message-input/next';
import MessageItem from '@/components/message-item';
import PdfSheet from '@/components/pdf-drawer';
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
import {
  useHandleMessageInputChange,
  useScrollToBottom,
} from '@/hooks/logic-hooks';
import {
  useFetchDialog,
  useGetChatSearchParams,
  useSetDialog,
} from '@/hooks/use-chat-request';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { IClientConversation } from '@/interfaces/database/chat';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { isEmpty, omit, trim } from 'lodash';
import { ListCheck, Plus, Trash2 } from 'lucide-react';
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useParams } from 'react-router';
import { z } from 'zod';
import {
  useGetSendButtonDisabled,
  useSendButtonDisabled,
} from '../../hooks/use-button-disabled';
import { useCreateConversationBeforeSendMessage } from '../../hooks/use-chat-url';
import { useCreateConversationBeforeUploadDocument } from '../../hooks/use-create-conversation';
import { useSendMessage } from '../../hooks/use-send-chat-message';
import {
  HandlePressEnterType,
  useSendSingleMessage,
  UseSendSingleMessageParameter,
} from '../../hooks/use-send-single-message';
import { useUploadFile } from '../../hooks/use-upload-file';
import { buildMessageItemReference } from '../../utils';
import { useAddChatBox } from '../use-add-box';
import { useShowInternet } from '../use-show-internet';
import { useSetDefaultModel } from './use-set-default-model';

type MultipleChatBoxProps = {
  controller: AbortController;
  chatBoxIds: string[];
  stopOutputMessage(): void;
  conversation: IClientConversation;
} & Pick<
  ReturnType<typeof useAddChatBox>,
  'removeChatBox' | 'addChatBox' | 'chatBoxIds'
>;

type ChatCardProps = {
  id: string;
  idx: number;
  conversation: IClientConversation;
  setLoading(id: string, loading: boolean): void;
} & Pick<
  MultipleChatBoxProps,
  'controller' | 'removeChatBox' | 'addChatBox' | 'chatBoxIds'
> &
  Pick<ReturnType<typeof useClickDrawer>, 'clickDocumentButton'> &
  UseSendSingleMessageParameter;

const ChatCard = forwardRef(function ChatCard(
  {
    controller,
    removeChatBox,
    id,
    idx,
    addChatBox,
    chatBoxIds,
    clickDocumentButton,
    conversation,
    value,
    setValue,
    files,
    clearFiles,
    setLoading,
  }: ChatCardProps,
  ref,
) {
  const { id: dialogId } = useParams();
  const { setDialog } = useSetDialog();

  const { removeMessageById, derivedMessages, handlePressEnter, sendLoading } =
    useSendSingleMessage({
      controller,
      value,
      setValue,
      files,
      clearFiles,
    });

  const { regenerateMessage } = useSendMessage(controller);

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

  useSetDefaultModel(form);

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

  useImperativeHandle(
    ref,
    (): HandlePressEnterType => (params) =>
      handlePressEnter({ ...params, ...form.getValues() }),
  );

  useEffect(() => {
    setLoading(id, sendLoading);
  }, [id, sendLoading, setLoading]);

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
                <p>{t('chat.applyModelConfigs')}</p>
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
  stopOutputMessage,
  conversation,
}: MultipleChatBoxProps) {
  const { createConversationBeforeSendMessage } =
    useCreateConversationBeforeSendMessage();

  const { createConversationBeforeUploadDocument } =
    useCreateConversationBeforeUploadDocument();
  const { conversationId } = useGetChatSearchParams();
  const disabled = useGetSendButtonDisabled();
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  const [chatBoxLoading, setChatBoxLoading] = useState<Map<string, boolean>>(
    new Map(),
  );

  const setLoading = useCallback((id: string, loading: boolean) => {
    setChatBoxLoading((prev) => {
      const newMap = new Map(prev);
      newMap.set(id, loading);
      return newMap;
    });
  }, []);

  const allChatBoxLoading = [...chatBoxLoading.values()];

  const showInternet = useShowInternet();

  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { handleUploadFile, isUploading, files, clearFiles, removeFile } =
    useUploadFile();
  const sendDisabled = useSendButtonDisabled(value);

  const boxesRef = useRef<Record<string, HandlePressEnterType>>({});

  const setFormRef = (id: string) => (ref: HandlePressEnterType) => {
    boxesRef.current[id] = ref;
  };

  const handlePressEnter = useCallback(
    async ({
      enableInternet,
      enableThinking,
    }: NextMessageInputOnPressEnterParameter) => {
      if (trim(value) === '') return;

      const data = await createConversationBeforeSendMessage(value);

      if (data === undefined) {
        return;
      }

      Object.values(boxesRef.current).forEach((box) => {
        box?.({
          enableInternet,
          enableThinking,
          ...data,
        });
      });
    },
    [createConversationBeforeSendMessage, value],
  );

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
            ref={setFormRef(id)}
            clickDocumentButton={clickDocumentButton}
            conversation={conversation}
            value={value}
            files={files}
            setValue={setValue}
            clearFiles={clearFiles}
            setLoading={setLoading}
          ></ChatCard>
        ))}
      </div>
      <div className="px-[20%]">
        <NextMessageInput
          disabled={disabled}
          sendDisabled={sendDisabled}
          sendLoading={allChatBoxLoading.some((loading) => loading)}
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
          showReasoning
          showInternet={showInternet}
          removeFile={removeFile}
          isUploading={isUploading}
        />
      </div>
      {visible && (
        <PdfSheet
          visible={visible}
          hideModal={hideModal}
          documentId={documentId}
          chunk={selectedChunk}
        ></PdfSheet>
      )}
    </section>
  );
}
