import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useTranslate } from '@/hooks/commonHooks';
import { useGetDocumentUrl } from '@/hooks/documentHooks';
import { useSelectFileThumbnails } from '@/hooks/knowledgeHook';
import { useSelectUserInfo } from '@/hooks/userSettingHook';
import { IReference, Message } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import classNames from 'classnames';
import { useMemo } from 'react';

import MarkdownContent from '@/pages/chat/markdown-content';
import { getExtension, isPdf } from '@/utils/documentUtils';
import { Avatar, Flex, List } from 'antd';
import NewDocumentLink from '../new-document-link';
import SvgIcon from '../svg-icon';
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

export default MessageItem;
