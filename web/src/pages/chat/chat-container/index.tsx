import MessageItem from '@/components/message-item';
import { MessageType } from '@/constants/chat';
import { Button, Flex, Spin } from 'antd';
import { useRef } from 'react';
import {
  useCreateConversationBeforeUploadDocument,
  useGetFileIcon,
  useGetSendButtonDisabled,
  useSendButtonDisabled,
  useSendNextMessage,
} from '../hooks';
import { buildMessageItemReference } from '../utils';

import MessageInput from '@/components/message-input';
import PdfDrawer from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import {
  useFetchNextConversation,
  useFetchNextDialog,
  useGetChatSearchParams,
} from '@/hooks/chat-hooks';
import { useScrollToBottom } from '@/hooks/logic-hooks';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { memo } from 'react';
import styles from './index.less';

interface IProps {
  controller: AbortController;
}

const ChatContainer = ({ controller }: IProps) => {
  const { conversationId } = useGetChatSearchParams();
  const { data: conversation } = useFetchNextConversation();
  const { data: currentDialog } = useFetchNextDialog();

  const messageContainerRef = useRef<HTMLDivElement>(null);

  const {
    value,
    ref,
    loading,
    sendLoading,
    derivedMessages,
    handleInputChange,
    handlePressEnter,
    regenerateMessage,
    removeMessageById,
    stopOutputMessage,
  } = useSendNextMessage(controller);

  // Use the scroll-to-bottom hook (only once!)
  const { scrollRef, isAtBottom } = useScrollToBottom(
    derivedMessages,
    messageContainerRef,
  );

  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();
  const disabled = useGetSendButtonDisabled();
  const sendDisabled = useSendButtonDisabled(value);
  useGetFileIcon();
  const { data: userInfo } = useFetchUserInfo();
  const { createConversationBeforeUploadDocument } =
    useCreateConversationBeforeUploadDocument();

  return (
    <>
      <Flex flex={1} className={styles.chatContainer} vertical>
        <Flex
          flex={1}
          vertical
          className={styles.messageContainer}
          ref={messageContainerRef}
          style={{ position: 'relative' }}
        >
          <div>
            <Spin spinning={loading}>
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
                      message: derivedMessages,
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
            </Spin>
          </div>
          {/* Attach scrollRef here */}
          <div ref={scrollRef} />
        </Flex>
        {/* Place the button here, above the input box */}
        {console.log('isAtBottom:', isAtBottom)}
        {!isAtBottom && (
          <Button
            type="primary"
            shape="round"
            style={{
              margin: '12px auto',
              display: 'block',
            }}
            onClick={() =>
              scrollRef.current?.scrollIntoView({ behavior: 'smooth' })
            }
          >
            Scroll to Bottom
          </Button>
        )}
        <MessageInput
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
      </Flex>
      <PdfDrawer
        visible={visible}
        hideModal={hideModal}
        documentId={documentId}
        chunk={selectedChunk}
      />
    </>
  );
};

export default memo(ChatContainer);
