import MessageItem from '@/components/message-item';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useSendButtonDisabled } from '@/pages/chat/hooks';
import { Button, Flex, Input, Spin } from 'antd';
import { forwardRef } from 'react';
import {
  useCreateSharedConversationOnMount,
  useSelectCurrentSharedConversation,
  useSendSharedMessage,
} from '../shared-hooks';
import { buildMessageItemReference } from '../utils';

import styles from './index.less';

const ChatContainer = () => {
  const { t } = useTranslate('chat');
  const { conversationId } = useCreateSharedConversationOnMount();
  const {
    currentConversation: conversation,
    addNewestConversation,
    removeLatestMessage,
    ref,
    loading,
    setCurrentConversation,
    addNewestAnswer,
  } = useSelectCurrentSharedConversation(conversationId);

  const {
    handlePressEnter,
    handleInputChange,
    value,
    loading: sendLoading,
  } = useSendSharedMessage(
    conversation,
    addNewestConversation,
    removeLatestMessage,
    setCurrentConversation,
    addNewestAnswer,
  );
  const sendDisabled = useSendButtonDisabled(value);

  return (
    <>
      <Flex flex={1} className={styles.chatContainer} vertical>
        <Flex flex={1} vertical className={styles.messageContainer}>
          <div>
            <Spin spinning={loading}>
              {conversation?.message?.map((message, i) => {
                return (
                  <MessageItem
                    key={message.id}
                    item={message}
                    nickname="You"
                    reference={buildMessageItemReference(conversation, message)}
                    loading={
                      message.role === MessageType.Assistant &&
                      sendLoading &&
                      conversation?.message.length - 1 === i
                    }
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
          //   disabled={disabled}
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
    </>
  );
};

export default forwardRef(ChatContainer);
