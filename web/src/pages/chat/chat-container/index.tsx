import { Button, Flex, Input } from 'antd';
import { ChangeEventHandler, useState } from 'react';

import { Message } from '@/interfaces/database/chat';
import classNames from 'classnames';
import { useFetchConversation, useSendMessage } from '../hooks';

import { MessageType } from '@/constants/chat';
import { IClientConversation } from '../interface';
import styles from './index.less';

const MessageItem = ({ item }: { item: Message }) => {
  return (
    <div
      className={classNames(styles.messageItem, {
        [styles.messageItemLeft]: item.role === MessageType.Assistant,
        [styles.messageItemRight]: item.role === MessageType.User,
      })}
    >
      <span className={styles.messageItemContent}>
        <div>{item.content}</div>
      </span>
    </div>
  );
};

const ChatContainer = () => {
  const [value, setValue] = useState('');
  const conversation: IClientConversation = useFetchConversation();
  const { sendMessage } = useSendMessage();

  const handlePressEnter = () => {
    console.info(value);
    sendMessage(value);
  };

  const handleInputChange: ChangeEventHandler<HTMLInputElement> = (e) => {
    setValue(e.target.value);
  };

  return (
    <Flex flex={1} className={styles.chatContainer} vertical>
      <Flex flex={1} vertical className={styles.messageContainer}>
        <div>
          {conversation?.message?.map((message) => (
            <MessageItem key={message.id} item={message}></MessageItem>
          ))}
        </div>
      </Flex>
      <Input
        size="large"
        placeholder="Message Resume Assistant..."
        value={value}
        suffix={
          <Button type="primary" onClick={handlePressEnter}>
            Send
          </Button>
        }
        onPressEnter={handlePressEnter}
        onChange={handleInputChange}
      />
    </Flex>
  );
};

export default ChatContainer;
