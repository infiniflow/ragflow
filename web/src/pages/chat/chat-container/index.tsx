import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import NewDocumentLink from '@/components/new-document-link';
import DocumentPreviewer from '@/components/pdf-previewer';
import { MessageType } from '@/constants/chat';
import { useSelectFileThumbnails } from '@/hooks/knowledgeHook';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import { IReference, Message } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { Avatar, Button, Drawer, Flex, Input, List, Spin } from 'antd';
import classNames from 'classnames';
import { useMemo } from 'react';
import {
  useClickDrawer,
  useFetchConversationOnMount,
  useGetFileIcon,
  useGetSendButtonDisabled,
  useSelectConversationLoading,
  useSendMessage,
} from '../hooks';
import MarkdownContent from '../markdown-content';

import SvgIcon from '@/components/svg-icon';
import { useTranslate } from '@/hooks/commonHooks';
import { useGetDocumentUrl } from '@/hooks/documentHooks';
import { getExtension, isPdf } from '@/utils/documentUtils';
import { buildMessageItemReference } from '../utils';
import styles from './index.less';

const MessageItem = ({
  item,
  reference,
  loading = false,
  clickDocumentButton,
}: {
  item: Message;
  reference: IReference;
  loading?: boolean;
  clickDocumentButton: (documentId: string, chunk: IChunk) => void;
}) => {
  const userInfo = useSelectUserInfo();
  const fileThumbnails = useSelectFileThumbnails();
  const getDocumentUrl = useGetDocumentUrl();
  const { t } = useTranslate('chat');

  const isAssistant = item.role === MessageType.Assistant;

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
              <MarkdownContent
                content={content}
                reference={reference}
                clickDocumentButton={clickDocumentButton}
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
  const {
    ref,
    currentConversation: conversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
  } = useFetchConversationOnMount();
  const {
    handleInputChange,
    handlePressEnter,
    value,
    loading: sendLoading,
  } = useSendMessage(
    conversation,
    addNewestConversation,
    removeLatestMessage,
    addNewestAnswer,
  );
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();
  const disabled = useGetSendButtonDisabled();
  useGetFileIcon();
  const loading = useSelectConversationLoading();
  const { t } = useTranslate('chat');

  return (
    <>
      <Flex flex={1} className={styles.chatContainer} vertical>
        <Flex flex={1} vertical className={styles.messageContainer}>
          <div>
            <Spin spinning={loading}>
              {conversation?.message?.map((message, i) => {
                return (
                  <MessageItem
                    loading={
                      message.role === MessageType.Assistant &&
                      sendLoading &&
                      conversation?.message.length - 1 === i
                    }
                    key={message.id}
                    item={message}
                    reference={buildMessageItemReference(conversation, message)}
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
          disabled={disabled}
          suffix={
            <Button
              type="primary"
              onClick={handlePressEnter}
              loading={sendLoading}
              disabled={disabled}
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
