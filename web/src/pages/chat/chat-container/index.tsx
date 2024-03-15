import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import Image from '@/components/image';
import NewDocumentLink from '@/components/new-document-link';
import DocumentPreviewer from '@/components/pdf-previewer';
import { MessageType } from '@/constants/chat';
import { useSelectFileThumbnails } from '@/hooks/knowledgeHook';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import { IReference, Message } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { InfoCircleOutlined } from '@ant-design/icons';
import {
  Avatar,
  Button,
  Drawer,
  Flex,
  Input,
  List,
  Popover,
  Skeleton,
  Space,
} from 'antd';
import classNames from 'classnames';
import { ChangeEventHandler, useCallback, useMemo, useState } from 'react';
import Markdown from 'react-markdown';
import reactStringReplace from 'react-string-replace';
import remarkGfm from 'remark-gfm';
import { visitParents } from 'unist-util-visit-parents';
import {
  useClickDrawer,
  useFetchConversationOnMount,
  useGetFileIcon,
  useSendMessage,
} from '../hooks';

import styles from './index.less';

const reg = /(#{2}\d+\${2})/g;

const getChunkIndex = (match: string) => Number(match.slice(2, -2));

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
  clickDocumentButton,
}: {
  item: Message;
  reference: IReference;
  clickDocumentButton: (documentId: string, chunk: IChunk) => void;
}) => {
  const userInfo = useSelectUserInfo();
  const fileThumbnails = useSelectFileThumbnails();

  const isAssistant = item.role === MessageType.Assistant;

  const handleDocumentButtonClick = useCallback(
    (documentId: string, chunk: IChunk) => () => {
      clickDocumentButton(documentId, chunk);
    },
    [clickDocumentButton],
  );

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
          <Popover
            placement="topRight"
            content={
              <Image
                id={chunkItem?.img_id}
                className={styles.referenceImagePreview}
              ></Image>
            }
          >
            <Image
              id={chunkItem?.img_id}
              className={styles.referenceChunkImage}
            ></Image>
          </Popover>
          <Space direction={'vertical'}>
            <div
              dangerouslySetInnerHTML={{
                __html: chunkItem?.content_with_weight,
              }}
              className={styles.chunkContentText}
            ></div>
            {documentId && (
              <Flex gap={'middle'}>
                <img src={fileThumbnails[documentId]} alt="" />
                <Button
                  type="link"
                  onClick={handleDocumentButtonClick(documentId, chunkItem)}
                >
                  {document?.doc_name}
                </Button>
              </Flex>
            )}
          </Space>
        </Flex>
      );
    },
    [reference, fileThumbnails, handleDocumentButtonClick],
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
            <Avatar
              size={40}
              src={
                userInfo.avatar ??
                'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png'
              }
            />
          ) : (
            <AssistantIcon></AssistantIcon>
          )}
          <Flex vertical gap={8} flex={1}>
            <b>{isAssistant ? '' : userInfo.nickname}</b>
            <div className={styles.messageText}>
              {item.content !== '' ? (
                <Markdown
                  rehypePlugins={[rehypeWrapReference]}
                  remarkPlugins={[remarkGfm]}
                  components={
                    {
                      'custom-typography': ({
                        children,
                      }: {
                        children: string;
                      }) => renderReference(children),
                    } as any
                  }
                >
                  {item.content}
                </Markdown>
              ) : (
                <Skeleton active className={styles.messageEmpty} />
              )}
            </div>
            {isAssistant && referenceDocumentList.length > 0 && (
              <List
                bordered
                dataSource={referenceDocumentList}
                renderItem={(item) => (
                  <List.Item>
                    {/* <SvgIcon name={getFileIcon(item.doc_name)}></SvgIcon> */}
                    <Flex gap={'middle'}>
                      <img src={fileThumbnails[item.doc_id]}></img>
                      <NewDocumentLink documentId={item.doc_id}>
                        {item.doc_name}
                      </NewDocumentLink>
                    </Flex>
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
  const {
    ref,
    currentConversation: conversation,
    addNewestConversation,
  } = useFetchConversationOnMount();
  const { sendMessage } = useSendMessage();
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  const loading = useOneNamespaceEffectsLoading('chatModel', [
    'completeConversation',
  ]);
  useGetFileIcon();

  const handlePressEnter = () => {
    if (!loading) {
      setValue('');
      addNewestConversation(value);
      sendMessage(value);
    }
  };

  const handleInputChange: ChangeEventHandler<HTMLInputElement> = (e) => {
    const value = e.target.value.trim();
    const nextValue = value.replaceAll('\\n', '\n');
    setValue(nextValue);
  };

  return (
    <>
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
                  clickDocumentButton={clickDocumentButton}
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
