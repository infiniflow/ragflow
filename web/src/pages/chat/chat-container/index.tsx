import MessageItem from '@/components/message-item';
import DocumentPreviewer from '@/components/pdf-previewer';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { Button, Drawer, Flex, Input, Spin } from 'antd';
import {
  useClickDrawer,
  useFetchConversationOnMount,
  useGetFileIcon,
  useGetSendButtonDisabled,
  useSelectConversationLoading,
  useSendButtonDisabled,
  useSendMessage,
} from '../hooks';
import { buildMessageItemReference } from '../utils';

import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import styles from './index.less';

const ChatContainer = () => {
  const {
    ref,
    currentConversation: conversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
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
  const loading = useSelectConversationLoading();
  const { t } = useTranslate('chat');
  const { data: userInfo } = useFetchUserInfo();

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
                  ></MessageItem>
                );
              })}
            </Spin>
          </div>
          <div ref={ref} />
        </Flex>
        <Input
          size="large"
          placeholder={t('sendPlaceholder')}
          value={value}
          disabled={disabled}
          suffix={
            <Button
              type="primary"
              onClick={handlePressEnter}
              loading={sendLoading}
              disabled={sendDisabled}
            >
              {t('send')}
            </Button>
          }
          onPressEnter={handlePressEnter}
          onChange={handleInputChange}
        />
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

export default ChatContainer;
