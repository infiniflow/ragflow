import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectFileThumbnails } from '@/hooks/knowledge-hooks';
import { IReference, Message } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import classNames from 'classnames';
import { useMemo } from 'react';

import MarkdownContent from '@/pages/chat/markdown-content';
import { getExtension } from '@/utils/document-util';
import { Avatar, Flex, List } from 'antd';
import NewDocumentLink from '../new-document-link';
import SvgIcon from '../svg-icon';
import styles from './index.less';

interface IProps {
  item: Message;
  reference: IReference;
  loading?: boolean;
  nickname?: string;
  avatar?: string;
  clickDocumentButton?: (documentId: string, chunk: IChunk) => void;
}

const MessageItem = ({
  item,
  reference,
  loading = false,
  avatar = '',
  nickname = '',
  clickDocumentButton,
}: IProps) => {
  const isAssistant = item.role === MessageType.Assistant;
  const { t } = useTranslate('chat');
  const fileThumbnails = useSelectFileThumbnails();

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
                avatar ??
                'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png'
              }
            />
          ) : (
            <AssistantIcon></AssistantIcon>
          )}
          <Flex vertical gap={8} flex={1}>
            <b>{isAssistant ? '' : nickname}</b>
            <div
              className={
                isAssistant ? styles.messageText : styles.messageUserText
              }
            >
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
                          documentId={item.doc_id}
                          documentName={item.doc_name}
                          prefix="document"
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

export default MessageItem;
