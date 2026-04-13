import { MessageType } from '@/constants/chat';
import {
  IMessage,
  IReference,
  IReferenceChunk,
  UploadResponseDataType,
} from '@/interfaces/database/chat';
import classNames from 'classnames';
import { memo, useCallback, useMemo } from 'react';

import { IRegenerateMessage, IRemoveMessageById } from '@/hooks/logic-hooks';
import { cn } from '@/lib/utils';
import {
  DocumentDownloadButton,
  extractDocumentDownloadInfos,
  mergeDocumentDownloadInfos,
  removeDocumentDownloadInfos,
} from '../document-download-button';
import MarkdownContent from '../markdown-content';
import { ReferenceDocumentList } from '../next-message-item/reference-document-list';
import { ReferenceImageList } from '../next-message-item/reference-image-list';
import { UploadedMessageFiles } from '../next-message-item/uploaded-message-files';
import { RAGFlowAvatar } from '../ragflow-avatar';
import SvgIcon from '../svg-icon';
import { useTheme } from '../theme-provider';
import { AssistantGroupButton, UserGroupButton } from './group-button';
import styles from './index.module.less';

interface IProps extends Partial<IRemoveMessageById>, IRegenerateMessage {
  item: IMessage;
  reference: IReference;
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
}

const MessageItem = ({
  item,
  reference,
  loading = false,
  avatar,
  avatarDialog,
  sendLoading = false,
  clickDocumentButton,
  index,
  removeMessageById,
  regenerateMessage,
  showLikeButton = true,
  showLoudspeaker = true,
  visibleAvatar = true,
}: IProps) => {
  const { theme } = useTheme();
  const isAssistant = item.role === MessageType.Assistant;
  const isUser = item.role === MessageType.User;

  const uploadedFiles = useMemo(() => {
    return item?.files ?? [];
  }, [item?.files]);

  const referenceDocumentList = useMemo(() => {
    return reference?.doc_aggs ?? [];
  }, [reference?.doc_aggs]);

  // Extract PDF download info from message content
  const documentDownloadInfos = useMemo(
    () =>
      mergeDocumentDownloadInfos(
        item.downloads,
        extractDocumentDownloadInfos(item.content),
      ),
    [item.content, item.downloads],
  );

  // If we have document download info, extract the remaining text
  const messageContent = useMemo(() => {
    if (!documentDownloadInfos.length) return item.content;

    // Remove the JSON part from the content to avoid showing it
    return removeDocumentDownloadInfos(item.content, documentDownloadInfos);
  }, [item.content, documentDownloadInfos]);

  const handleRegenerateMessage = useCallback(() => {
    regenerateMessage?.(item);
  }, [regenerateMessage, item]);

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
          className={classNames(styles.messageItemContent, 'group', {
            [styles.messageItemContentReverse]: item.role === MessageType.User,
          })}
        >
          {visibleAvatar &&
            (item.role === MessageType.User ? (
              <RAGFlowAvatar
                className="size-10"
                avatar={avatar ?? '/logo.svg'}
                isPerson
              />
            ) : avatarDialog ? (
              <RAGFlowAvatar
                className="size-10"
                avatar={avatarDialog}
                isPerson
              />
            ) : (
              <SvgIcon
                name={'assistant'}
                width={'100%'}
                className={cn('size-10 fill-current')}
              ></SvgIcon>
            ))}

          <section className="flex min-w-0 gap-2 flex-1 flex-col">
            {isAssistant ? (
              index !== 0 && (
                <AssistantGroupButton
                  messageId={item.id}
                  content={messageContent}
                  prompt={item.prompt}
                  showLikeButton={showLikeButton}
                  audioBinary={item.audio_binary}
                  showLoudspeaker={showLoudspeaker}
                ></AssistantGroupButton>
              )
            ) : (
              <UserGroupButton
                content={messageContent}
                messageId={item.id}
                removeMessageById={removeMessageById}
                regenerateMessage={regenerateMessage && handleRegenerateMessage}
                sendLoading={sendLoading}
              ></UserGroupButton>
            )}
            {/* Show message content if there's any text besides the download */}
            {messageContent && (
              <div
                className={cn(
                  isAssistant
                    ? theme === 'dark'
                      ? styles.messageTextDark
                      : styles.messageText
                    : styles.messageUserText,
                  { '!bg-bg-card': !isAssistant },
                )}
              >
                <MarkdownContent
                  loading={loading}
                  content={messageContent}
                  reference={reference}
                  clickDocumentButton={clickDocumentButton}
                ></MarkdownContent>
              </div>
            )}
            {isAssistant && (
              <ReferenceImageList
                referenceChunks={reference.chunks}
                messageContent={messageContent}
              ></ReferenceImageList>
            )}
            {isAssistant && referenceDocumentList.length > 0 && (
              <ReferenceDocumentList
                list={referenceDocumentList}
              ></ReferenceDocumentList>
            )}
            {isUser &&
              Array.isArray(uploadedFiles) &&
              uploadedFiles.length > 0 && (
                <UploadedMessageFiles
                  files={uploadedFiles as UploadResponseDataType[]}
                ></UploadedMessageFiles>
              )}
            {documentDownloadInfos.length > 0 && (
              <div className="mt-3 space-y-3">
                {documentDownloadInfos.map((downloadInfo, index) => (
                  <div key={`${downloadInfo.filename}-${index}`}>
                    {index > 0 && <div className="my-6 h-px bg-border" />}
                    <DocumentDownloadButton downloadInfo={downloadInfo} />
                  </div>
                ))}
              </div>
            )}
          </section>
        </div>
      </section>
    </div>
  );
};

export default memo(MessageItem);
