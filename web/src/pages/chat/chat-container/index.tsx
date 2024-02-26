import { Avatar, Button, Flex, Input } from 'antd';
import { ChangeEventHandler, useState } from 'react';

import { Message } from '@/interfaces/database/chat';
import classNames from 'classnames';
import { useFetchConversation, useSendMessage } from '../hooks';

import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { IClientConversation } from '../interface';

import { useSelectUserInfo } from '@/hooks/userSettingHook';
import styles from './index.less';

const MessageItem = ({ item }: { item: Message }) => {
  const userInfo = useSelectUserInfo();
  return (
    <div
      className={classNames(styles.messageItem, {
        [styles.messageItemLeft]: item.role === MessageType.Assistant,
        [styles.messageItemRight]: item.role === MessageType.User,
      })}
    >
      <section
        className={classNames(styles.messageItemSection, {
          [styles.messageItemSectionLeft]: item.role === MessageType.Assistant,
          [styles.messageItemSectionRight]: item.role === MessageType.User,
        })}
      >
        <div
          className={classNames(styles.messageItemContent, {
            [styles.messageItemContentReverse]: item.role === MessageType.User,
          })}
        >
          {item.role === MessageType.User ? (
            userInfo.avatar ?? (
              <Avatar
                size={40}
                src={
                  userInfo.avatar ??
                  'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png'
                }
              />
            )
          ) : (
            <AssistantIcon></AssistantIcon>
          )}
          <Flex vertical gap={8} flex={1}>
            <b>
              {item.role === MessageType.Assistant ? 'Resume Assistant' : 'You'}
            </b>
            <div className={styles.messageText}>{item.content}</div>
          </Flex>
        </div>
      </section>
    </div>
  );
};

const ChatContainer = () => {
  const [value, setValue] = useState('');
  const conversation: IClientConversation = useFetchConversation();
  const { sendMessage } = useSendMessage();
  const loading = useOneNamespaceEffectsLoading('chatModel', [
    'completeConversation',
    'getConversation',
  ]);

  const handlePressEnter = () => {
    setValue('');
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
          <Button type="primary" onClick={handlePressEnter} loading={loading}>
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
