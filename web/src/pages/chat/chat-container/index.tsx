import MessageItem from '@/components/message-item';
import DocumentPreviewer from '@/components/pdf-previewer';
import { MessageType } from '@/constants/chat';
import { Drawer, Flex, Spin } from 'antd';
import {
  useClickDrawer,
  useCreateConversationBeforeUploadDocument,
  useFetchConversationOnMount,
  useGetFileIcon,
  useGetSendButtonDisabled,
  useSendButtonDisabled,
  useSendMessage,
} from '../hooks';
import { buildMessageItemReference } from '../utils';

import MessageInput from '@/components/message-input';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { memo } from 'react';
import styles from './index.less';

const ChatContainer = () => {
  const {
    ref,
    currentConversation: conversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
    conversationId,
    loading,
    removeMessageById,
  } = useFetchConversationOnMount();
  const {
    handleInputChange,
    handlePressEnter,
    value,
    loading: sendLoading,
  } = useSendMessage(
    conversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
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
        <Flex flex={1} vertical className={styles.messageContainer}>
          <div>
            <Spin spinning={loading}>
              {conversation?.message?.map((message, i) => {
                return (
                  <MessageItem
                    loading={
                      message.role === MessageType.Assistant &&
                      sendLoading &&
                      conversation?.message.length - 1 === i
                    }
                    key={message.id}
                    item={message}
                    nickname={userInfo.nickname}
                    avatar={userInfo.avatar}
                    reference={buildMessageItemReference(conversation, message)}
                    clickDocumentButton={clickDocumentButton}
                    index={i}
                    removeMessageById={removeMessageById}
                  ></MessageItem>
                );
              })}
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
      <Drawer
        title="Document Previewer"
        onClose={hideModal}
        open={visible}
        width={'50vw'}
      >
        <DocumentPreviewer
          documentId={documentId}
          chunk={selectedChunk}
          visible={visible}
        ></DocumentPreviewer>
      </Drawer>
    </>
  );
};

export default memo(ChatContainer);
