import MessageItem from '@/components/message-item';
import DocumentPreviewer from '@/components/pdf-previewer';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/commonHooks';
import {
  useClickDrawer,
  useFetchConversationOnMount,
  useGetFileIcon,
  useGetSendButtonDisabled,
  useSelectConversationLoading,
  useSendMessage,
} from '@/pages/chat/hooks';
import { buildMessageItemReference } from '@/pages/chat/utils';
import { Button, Drawer, Flex, Input, Spin } from 'antd';

import styles from './index.less';

const FlowChatBox = () => {
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
  useGetFileIcon();
  const loading = useSelectConversationLoading();
  const { t } = useTranslate('chat');

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
              disabled={disabled}
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

export default FlowChatBox;
