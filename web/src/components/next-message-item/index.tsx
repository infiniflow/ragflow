import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { IReferenceChunk, IReferenceObject } from '@/interfaces/database/chat';
import classNames from 'classnames';
import {
  PropsWithChildren,
  memo,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';

import {
  useFetchDocumentInfosByIds,
  useFetchDocumentThumbnailsByIds,
} from '@/hooks/document-hooks';
import { IRegenerateMessage, IRemoveMessageById } from '@/hooks/logic-hooks';
import { cn } from '@/lib/utils';
import { IMessage } from '@/pages/chat/interface';
import { getExtension, isImage } from '@/utils/document-util';
import { Avatar, Button, Flex, List, Space, Typography } from 'antd';
import { isEmpty } from 'lodash';
import FileIcon from '../file-icon';
import IndentedTreeModal from '../indented-tree/modal';
import NewDocumentLink from '../new-document-link';
import MarkdownContent from '../next-markdown-content';
import { useTheme } from '../theme-provider';
import { AssistantGroupButton, UserGroupButton } from './group-button';
import styles from './index.less';
import { ReferenceDocumentList } from './reference-document-list';

const { Text } = Typography;

interface IProps
  extends Partial<IRemoveMessageById>,
    IRegenerateMessage,
    PropsWithChildren {
  item: IMessage;
  reference?: IReferenceObject;
  loading?: boolean;
  sendLoading?: boolean;
  visibleAvatar?: boolean;
  nickname?: string;
  avatar?: string;
  avatarDialog?: string | null;
  clickDocumentButton?: (documentId: string, chunk: IReferenceChunk) => void;
  index: number;
  showLikeButton?: boolean;
  showLoudspeaker?: boolean;
  showLog?: boolean;
}

function MessageItem({
  item,
  reference,
  loading = false,
  avatar,
  avatarDialog,
  sendLoading = false,
  clickDocumentButton,
  removeMessageById,
  regenerateMessage,
  showLikeButton = true,
  showLoudspeaker = true,
  visibleAvatar = true,
  children,
  showLog,
}: IProps) {
  const { theme } = useTheme();
  const isAssistant = item.role === MessageType.Assistant;
  const isUser = item.role === MessageType.User;
  const { data: documentList, setDocumentIds } = useFetchDocumentInfosByIds();
  const { data: documentThumbnails, setDocumentIds: setIds } =
    useFetchDocumentThumbnailsByIds();
  const { visible, hideModal, showModal } = useSetModalState();
  const [clickedDocumentId, setClickedDocumentId] = useState('');

  const referenceDocuments = useMemo(() => {
    const docs = reference?.doc_aggs ?? {};

    return Object.values(docs);
  }, [reference?.doc_aggs]);

  const handleUserDocumentClick = useCallback(
    (id: string) => () => {
      setClickedDocumentId(id);
      showModal();
    },
    [showModal],
  );

  const handleRegenerateMessage = useCallback(() => {
    regenerateMessage?.(item);
  }, [regenerateMessage, item]);

  useEffect(() => {
    const ids = item?.doc_ids ?? [];
    if (ids.length) {
      setDocumentIds(ids);
      const documentIds = ids.filter((x) => !(x in documentThumbnails));
      if (documentIds.length) {
        setIds(documentIds);
      }
    }
  }, [item.doc_ids, setDocumentIds, setIds, documentThumbnails]);

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
          {visibleAvatar &&
            (item.role === MessageType.User ? (
              <Avatar size={40} src={avatar ?? '/logo.svg'} />
            ) : avatarDialog ? (
              <Avatar size={40} src={avatarDialog} />
            ) : (
              <AssistantIcon />
            ))}

          <Flex vertical gap={8} flex={1}>
            <Space>
              {isAssistant ? (
                <AssistantGroupButton
                  messageId={item.id}
                  content={item.content}
                  prompt={item.prompt}
                  showLikeButton={showLikeButton}
                  audioBinary={item.audio_binary}
                  showLoudspeaker={showLoudspeaker}
                  showLog={showLog}
                ></AssistantGroupButton>
              ) : (
                <UserGroupButton
                  content={item.content}
                  messageId={item.id}
                  removeMessageById={removeMessageById}
                  regenerateMessage={
                    regenerateMessage && handleRegenerateMessage
                  }
                  sendLoading={sendLoading}
                ></UserGroupButton>
              )}

              {/* <b>{isAssistant ? '' : nickname}</b> */}
            </Space>
            <div
              className={cn({
                [theme === 'dark'
                  ? styles.messageTextDark
                  : styles.messageText]: isAssistant,
                [styles.messageUserText]: !isAssistant,
                'bg-background-card': !isAssistant,
              })}
            >
              {item.data ? (
                children
              ) : sendLoading && isEmpty(item.content) ? (
                'searching...'
              ) : (
                <MarkdownContent
                  loading={loading}
                  content={item.content}
                  reference={reference}
                  clickDocumentButton={clickDocumentButton}
                ></MarkdownContent>
              )}
            </div>
            {isAssistant && referenceDocuments.length > 0 && (
              <ReferenceDocumentList
                list={referenceDocuments}
              ></ReferenceDocumentList>
            )}
            {isUser && documentList.length > 0 && (
              <List
                bordered
                dataSource={documentList}
                renderItem={(item) => {
                  // TODO:
                  // const fileThumbnail =
                  //   documentThumbnails[item.id] || documentThumbnails[item.id];
                  const fileExtension = getExtension(item.name);
                  return (
                    <List.Item>
                      <Flex gap={'small'} align="center">
                        <FileIcon id={item.id} name={item.name}></FileIcon>

                        {isImage(fileExtension) ? (
                          <NewDocumentLink
                            documentId={item.id}
                            documentName={item.name}
                            prefix="document"
                          >
                            {item.name}
                          </NewDocumentLink>
                        ) : (
                          <Button
                            type={'text'}
                            onClick={handleUserDocumentClick(item.id)}
                          >
                            <Text
                              style={{ maxWidth: '40vw' }}
                              ellipsis={{ tooltip: item.name }}
                            >
                              {item.name}
                            </Text>
                          </Button>
                        )}
                      </Flex>
                    </List.Item>
                  );
                }}
              />
            )}
          </Flex>
        </div>
      </section>
      {visible && (
        <IndentedTreeModal
          visible={visible}
          hideModal={hideModal}
          documentId={clickedDocumentId}
        ></IndentedTreeModal>
      )}
    </div>
  );
}

export default memo(MessageItem);
