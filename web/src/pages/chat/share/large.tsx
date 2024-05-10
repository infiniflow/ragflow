import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/commonHooks';
import { Message } from '@/interfaces/database/chat';
import { Avatar, Button, Flex, Input, Skeleton, Spin } from 'antd';
import classNames from 'classnames';
import { useSelectConversationLoading } from '../hooks';

import HightLightMarkdown from '@/components/highlight-markdown';
import React, { ChangeEventHandler, forwardRef } from 'react';
import { IClientConversation } from '../interface';
import styles from './index.less';

const MessageItem = ({ item }: { item: Message }) => {
  const isAssistant = item.role === MessageType.Assistant;

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
            <Avatar
              size={40}
              src={
                'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png'
              }
            />
          ) : (
            <AssistantIcon></AssistantIcon>
          )}
          <Flex vertical gap={8} flex={1}>
            <b>{isAssistant ? '' : 'You'}</b>
            <div className={styles.messageText}>
              {item.content !== '' ? (
                <HightLightMarkdown>{item.content}</HightLightMarkdown>
              ) : (
                <Skeleton active className={styles.messageEmpty} />
              )}
            </div>
          </Flex>
        </div>
      </section>
    </div>
  );
};

interface IProps {
  handlePressEnter(): void;
  handleInputChange: ChangeEventHandler<HTMLInputElement>;
  value: string;
  loading: boolean;
  sendLoading: boolean;
  conversation: IClientConversation;
  ref: React.LegacyRef<any>;
}

const ChatContainer = (
  {
    handlePressEnter,
    handleInputChange,
    value,
    loading: sendLoading,
    conversation,
  }: IProps,
  ref: React.LegacyRef<any>,
) => {
  const loading = useSelectConversationLoading();
  const { t } = useTranslate('chat');

  return (
    <>
      <Flex flex={1} className={styles.chatContainer} vertical>
        <Flex flex={1} vertical className={styles.messageContainer}>
          <div>
            <Spin spinning={loading}>
              {conversation?.message?.map((message) => {
                return (
                  <MessageItem key={message.id} item={message}></MessageItem>
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
              //   disabled={disabled}
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
