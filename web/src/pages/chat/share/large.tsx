import MessageInput from '@/components/message-input';
import MessageItem from '@/components/message-item';
import { MessageType, SharedFrom } from '@/constants/chat';
import { useSendButtonDisabled } from '@/pages/chat/hooks';
import { Flex, Spin } from 'antd';
import { forwardRef } from 'react';
import {
  useCreateSharedConversationOnMount,
  useGetSharedChatSearchParams,
  useSelectCurrentSharedConversation,
  useSendSharedMessage,
} from '../shared-hooks';
import { buildMessageItemReference } from '../utils';
import styles from './index.less';

const ChatContainer = () => {
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
  const { from } = useGetSharedChatSearchParams();

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

        <MessageInput
          isShared
          value={value}
          disabled={false}
          sendDisabled={sendDisabled}
          conversationId={conversationId}
          onInputChange={handleInputChange}
          onPressEnter={handlePressEnter}
          sendLoading={sendLoading}
          uploadMethod="external_upload_and_parse"
          showUploadIcon={from === SharedFrom.Chat}
        ></MessageInput>
      </Flex>
    </>
  );
};

export default forwardRef(ChatContainer);
