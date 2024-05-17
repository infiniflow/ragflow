import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/commonHooks';
import { IReference, Message } from '@/interfaces/database/chat';
import { Avatar, Button, Flex, Input, List, Spin } from 'antd';
import classNames from 'classnames';

import NewDocumentLink from '@/components/new-document-link';
import SvgIcon from '@/components/svg-icon';
import { useGetDocumentUrl } from '@/hooks/documentHooks';
import { useSelectFileThumbnails } from '@/hooks/knowledgeHook';
import { getExtension, isPdf } from '@/utils/documentUtils';
import { forwardRef, useMemo } from 'react';
import MarkdownContent from '../markdown-content';
import {
  useCreateSharedConversationOnMount,
  useSelectCurrentSharedConversation,
  useSendSharedMessage,
} from '../shared-hooks';
import { buildMessageItemReference } from '../utils';
import styles from './index.less';

const MessageItem = ({
  item,
  reference,
  loading = false,
}: {
  item: Message;
  reference: IReference;
  loading?: boolean;
}) => {
  const isAssistant = item.role === MessageType.Assistant;
  const { t } = useTranslate('chat');
  const fileThumbnails = useSelectFileThumbnails();
  const getDocumentUrl = useGetDocumentUrl();

  const referenceDocumentList = useMemo(() => {
    return reference?.doc_aggs ?? [];
  }, [reference?.doc_aggs]);

  const content = useMemo(() => {
    let text = item.content;
    if (text === '') {
      text = t('searching');
    }
    return loading ? text?.concat('~~2$$') : text;
  }, [item.content, loading, t]);

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
              <MarkdownContent
                reference={reference}
                clickDocumentButton={() => {}}
                content={content}
              ></MarkdownContent>
            </div>
            {isAssistant && referenceDocumentList.length > 0 && (
              <List
                bordered
                dataSource={referenceDocumentList}
                renderItem={(item) => {
                  const fileThumbnail = fileThumbnails[item.doc_id];
                  const fileExtension = getExtension(item.doc_name);
                  return (
                    <List.Item>
                      <Flex gap={'small'} align="center">
                        {fileThumbnail ? (
                          <img
                            src={fileThumbnail}
                            className={styles.thumbnailImg}
                          ></img>
                        ) : (
                          <SvgIcon
                            name={`file-icon/${fileExtension}`}
                            width={24}
                          ></SvgIcon>
                        )}

                        <NewDocumentLink
                          link={getDocumentUrl(item.doc_id)}
                          preventDefault={!isPdf(item.doc_name)}
                        >
                          {item.doc_name}
                        </NewDocumentLink>
                      </Flex>
                    </List.Item>
                  );
                }}
              />
            )}
          </Flex>
        </div>
      </section>
    </div>
  );
};

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
