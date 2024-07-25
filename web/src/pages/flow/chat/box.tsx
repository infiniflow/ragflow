import MessageItem from '@/components/message-item';
import DocumentPreviewer from '@/components/pdf-previewer';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useClickDrawer, useGetFileIcon } from '@/pages/chat/hooks';
import { buildMessageItemReference } from '@/pages/chat/utils';
import { Button, Drawer, Flex, Input, Spin } from 'antd';

import { useSelectCurrentMessages, useSendMessage } from './hooks';

import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import styles from './index.less';

const FlowChatBox = () => {
  const {
    ref,
    currentMessages,
    reference,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
    loading,
  } = useSelectCurrentMessages();

  const {
    handleInputChange,
    handlePressEnter,
    value,
    loading: sendLoading,
  } = useSendMessage(addNewestQuestion, removeLatestMessage, addNewestAnswer);
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();
  useGetFileIcon();
  const { t } = useTranslate('chat');
  const { data: userInfo } = useFetchUserInfo();

  return (
    <>
      <Flex flex={1} className={styles.chatContainer} vertical>
        <Flex flex={1} vertical className={styles.messageContainer}>
          <div>
            <Spin spinning={loading}>
              {currentMessages?.map((message, i) => {
                return (
                  <MessageItem
                    loading={
                      message.role === MessageType.Assistant &&
                      sendLoading &&
                      currentMessages.length - 1 === i
                    }
                    key={message.id}
                    nickname={userInfo.nickname}
                    avatar={userInfo.avatar}
                    item={message}
                    reference={buildMessageItemReference(
                      { message: currentMessages, reference },
                      message,
                    )}
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
          suffix={
            <Button
              type="primary"
              onClick={handlePressEnter}
              loading={sendLoading}
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
        mask={false}
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
