import MessageItem from '@/components/message-item';
import { MessageType } from '@/constants/chat';
import { Flex, Spin } from 'antd';
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
  useGetChatSearchParams,
} from '@/hooks/chat-hooks';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { memo } from 'react';
import { ConversationContext } from '../context';
import styles from './index.less';

const ChatContainer = () => {
  const { conversationId } = useGetChatSearchParams();
  const { data: conversation } = useFetchNextConversation();

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
    redirectToNewConversation,
  } = useSendNextMessage();

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
        <Flex flex={1} vertical className={styles.messageContainer}>
          <div>
            <Spin spinning={loading}>
              <ConversationContext.Provider value={redirectToNewConversation}>
                {derivedMessages?.map((message, i) => {
                  return (
                    <MessageItem
                      loading={
                        message.role === MessageType.Assistant &&
                        sendLoading &&
                        derivedMessages.length - 1 === i
                      }
                      key={message.id}
                      item={message}
                      nickname={userInfo.nickname}
                      avatar={userInfo.avatar}
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
                    ></MessageItem>
                  );
                })}
              </ConversationContext.Provider>
            </Spin>
          </div>
          <div ref={ref} />
        </Flex>
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
        ></MessageInput>
      </Flex>
      <PdfDrawer
        visible={visible}
        hideModal={hideModal}
        documentId={documentId}
        chunk={selectedChunk}
      ></PdfDrawer>
    </>
  );
};

export default memo(ChatContainer);
