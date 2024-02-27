import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import { IReference, Message } from '@/interfaces/database/chat';
import {
  Avatar,
  Button,
  Flex,
  Input,
  List,
  Popover,
  Space,
  Typography,
} from 'antd';
import classNames from 'classnames';
import { ChangeEventHandler, useCallback, useMemo, useState } from 'react';
import reactStringReplace from 'react-string-replace';
import {
  useFetchConversation,
  useGetFileIcon,
  useScrollToBottom,
  useSendMessage,
} from '../hooks';
import { IClientConversation } from '../interface';

import Image from '@/components/image';
import NewDocumentLink from '@/components/new-document-link';
import { InfoCircleOutlined } from '@ant-design/icons';
import Markdown from 'react-markdown';
import { visitParents } from 'unist-util-visit-parents';
import styles from './index.less';

const reg = /(#{2}\d+\${2})/g;

const getChunkIndex = (match: string) => Number(match.slice(2, 3));

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

const MessageItem = ({
  item,
  reference,
}: {
  item: Message;
  reference: IReference;
}) => {
  const userInfo = useSelectUserInfo();

  const isAssistant = item.role === MessageType.Assistant;

  const getFileIcon = useGetFileIcon();

  const getPopoverContent = useCallback(
    (chunkIndex: number) => {
      const chunks = reference?.chunks ?? [];
      const chunkItem = chunks[chunkIndex];
      const document = reference?.doc_aggs.find(
        (x) => x?.doc_id === chunkItem?.doc_id,
      );
      const documentId = document?.doc_id;
      return (
        <Flex
          key={chunkItem?.chunk_id}
          gap={10}
          className={styles.referencePopoverWrapper}
        >
          <Image
            id={chunkItem?.img_id}
            className={styles.referenceChunkImage}
          ></Image>
          <Space direction={'vertical'}>
            <div>{chunkItem?.content_with_weight}</div>
            {documentId && (
              <NewDocumentLink documentId={documentId}>
                {document?.doc_name}
              </NewDocumentLink>
            )}
          </Space>
        </Flex>
      );
    },
    [reference],
  );

  const renderReference = useCallback(
    (text: string) => {
      return reactStringReplace(text, reg, (match, i) => {
        const chunkIndex = getChunkIndex(match);
        return (
          <Popover content={getPopoverContent(chunkIndex)}>
            <InfoCircleOutlined key={i} className={styles.referenceIcon} />
          </Popover>
        );
      });
    },
    [getPopoverContent],
  );

  const referenceDocumentList = useMemo(() => {
    return reference?.doc_aggs ?? [];
  }, [reference?.doc_aggs]);

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
            <b>{isAssistant ? 'Resume Assistant' : 'You'}</b>
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
            {isAssistant && referenceDocumentList.length > 0 && (
              <List
                bordered
                dataSource={referenceDocumentList}
                renderItem={(item) => (
                  <List.Item>
                    <Typography.Text mark>
                      {/* <SvgIcon name={getFileIcon(item.doc_name)}></SvgIcon> */}
                    </Typography.Text>
                    <NewDocumentLink documentId={item.doc_id}>
                      {item.doc_name}
                    </NewDocumentLink>
                  </List.Item>
                )}
              />
            )}
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
  const ref = useScrollToBottom();
  useGetFileIcon();

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
          {conversation?.message?.map((message) => {
            const assistantMessages = conversation?.message
              ?.filter((x) => x.role === MessageType.Assistant)
              .slice(1);
            const referenceIndex = assistantMessages.findIndex(
              (x) => x.id === message.id,
            );
            const reference = conversation.reference[referenceIndex];
            return (
              <MessageItem
                key={message.id}
                item={message}
                reference={reference}
              ></MessageItem>
            );
          })}
        </div>
        <div ref={ref} />
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
