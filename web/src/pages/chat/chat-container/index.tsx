import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import { IReference, Message } from '@/interfaces/database/chat';
import { Avatar, Button, Flex, Input, Popover } from 'antd';
import classNames from 'classnames';
import { ChangeEventHandler, useCallback, useMemo, useState } from 'react';
import reactStringReplace from 'react-string-replace';
import { useFetchConversation, useSendMessage } from '../hooks';
import { IClientConversation } from '../interface';

import { InfoCircleOutlined } from '@ant-design/icons';
import Markdown from 'react-markdown';
import { visitParents } from 'unist-util-visit-parents';
import styles from './index.less';

const rehypeWrapReference = () => {
  return function wrapTextTransform(tree: any) {
    visitParents(tree, 'text', (node, ancestors) => {
      if (ancestors.at(-1).tagName !== 'custom-typography') {
        node.type = 'element';
        node.tagName = 'custom-typography';
        node.properties = {};
        node.children = [{ type: 'text', value: node.value }];
      }
    });
  };
};

const MessageItem = ({ item }: { item: Message; references: IReference[] }) => {
  const userInfo = useSelectUserInfo();

  const popoverContent = useMemo(
    () => (
      <div>
        <p>Content</p>
        <p>Content</p>
      </div>
    ),
    [],
  );

  const renderReference = useCallback(
    (text: string) => {
      return reactStringReplace(text, /#{2}\d{1,}\${2}/g, (match, i) => {
        return (
          <Popover content={popoverContent}>
            <InfoCircleOutlined key={i} className={styles.referenceIcon} />
          </Popover>
        );
      });
    },
    [popoverContent],
  );

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
            <div className={styles.messageText}>
              <Markdown
                rehypePlugins={[rehypeWrapReference]}
                components={
                  {
                    'custom-typography': ({ children }: { children: string }) =>
                      renderReference(children),
                  } as any
                }
              >
                {item.content}
              </Markdown>
            </div>
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
            <MessageItem
              key={message.id}
              item={message}
              references={conversation.reference}
            ></MessageItem>
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
