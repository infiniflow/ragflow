import { ReactComponent as AssistantIcon } from '@/assets/svg/assistant.svg';
import { MessageType } from '@/constants/chat';
import { IReferenceChunk, IReferenceObject } from '@/interfaces/database/chat';
import classNames from 'classnames';
import {
  PropsWithChildren,
  memo,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';

import { IRegenerateMessage, IRemoveMessageById } from '@/hooks/logic-hooks';
import { INodeEvent, MessageEventType } from '@/hooks/use-send-message';
import { cn } from '@/lib/utils';
import { AgentChatContext } from '@/pages/agent/context';
import { WorkFlowTimeline } from '@/pages/agent/log-sheet/workflow-timeline';
import { IMessage } from '@/pages/chat/interface';
import { downloadFile } from '@/services/file-manager-service';
import { downloadFileFromBlob } from '@/utils/file-util';
import { isEmpty } from 'lodash';
import { Atom, ChevronDown, ChevronUp, Download } from 'lucide-react';
import MarkdownContent from '../next-markdown-content';
import { RAGFlowAvatar } from '../ragflow-avatar';
import { useTheme } from '../theme-provider';
import { Button } from '../ui/button';
import { AssistantGroupButton, UserGroupButton } from './group-button';
import styles from './index.less';
import { ReferenceDocumentList } from './reference-document-list';
import { UploadedMessageFiles } from './uploaded-message-files';

interface IProps
  extends Partial<IRemoveMessageById>,
    IRegenerateMessage,
    PropsWithChildren {
  item: IMessage;
  conversationId?: string;
  currentEventListWithoutMessageById?: (messageId: string) => INodeEvent[];
  setCurrentMessageId?: (messageId: string) => void;
  reference?: IReferenceObject;
  loading?: boolean;
  sendLoading?: boolean;
  visibleAvatar?: boolean;
  nickname?: string;
  avatar?: string;
  avatarDialog?: string | null;
  agentName?: string;
  clickDocumentButton?: (documentId: string, chunk: IReferenceChunk) => void;
  index: number;
  showLikeButton?: boolean;
  showLoudspeaker?: boolean;
  showLog?: boolean;
  isShare?: boolean;
}

function MessageItem({
  item,
  conversationId,
  currentEventListWithoutMessageById,
  setCurrentMessageId,
  reference,
  loading = false,
  avatar,
  avatarDialog,
  agentName,
  sendLoading = false,
  clickDocumentButton,
  removeMessageById,
  regenerateMessage,
  showLikeButton = true,
  showLoudspeaker = true,
  visibleAvatar = true,
  children,
  showLog,
  isShare,
}: IProps) {
  const { theme } = useTheme();
  const isAssistant = item.role === MessageType.Assistant;
  const isUser = item.role === MessageType.User;
  const [showThinking, setShowThinking] = useState(false);
  const { setLastSendLoadingFunc } = useContext(AgentChatContext);

  useEffect(() => {
    if (typeof setLastSendLoadingFunc === 'function') {
      setLastSendLoadingFunc(loading, item.id);
    }
  }, [loading, setLastSendLoadingFunc, item.id]);

  const referenceDocuments = useMemo(() => {
    const docs = reference?.doc_aggs ?? {};

    return Object.values(docs);
  }, [reference?.doc_aggs]);

  const handleRegenerateMessage = useCallback(() => {
    regenerateMessage?.(item);
  }, [regenerateMessage, item]);

  useEffect(() => {
    if (typeof setCurrentMessageId === 'function') {
      setCurrentMessageId(item.id);
    }
  }, [item.id, setCurrentMessageId]);

  const startedNodeList = useCallback(
    (item: IMessage) => {
      const finish = currentEventListWithoutMessageById?.(item.id)?.some(
        (item) => item.event === MessageEventType.WorkflowFinished,
      );
      return !finish && loading;
    },
    [currentEventListWithoutMessageById, loading],
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
          {visibleAvatar &&
            (item.role === MessageType.User ? (
              <RAGFlowAvatar avatar={avatar ?? '/logo.svg'} />
            ) : avatarDialog || agentName ? (
              <RAGFlowAvatar
                avatar={avatarDialog as string}
                name={agentName}
                isPerson
              />
            ) : (
              <AssistantIcon />
            ))}
          <section className="flex-col gap-2 flex-1">
            <div className="flex justify-between items-center">
              {isShare && isAssistant && (
                <Button
                  variant={'transparent'}
                  onClick={() => setShowThinking((think) => !think)}
                >
                  <div className="flex items-center gap-1">
                    <div className="">
                      <Atom
                        className={startedNodeList(item) ? 'animate-spin' : ''}
                      />
                    </div>
                    Thinking
                    {showThinking ? <ChevronUp /> : <ChevronDown />}
                  </div>
                </Button>
              )}
              <div className="space-x-1">
                {isAssistant ? (
                  <>
                    {isShare && !sendLoading && !isEmpty(item.content) && (
                      <AssistantGroupButton
                        messageId={item.id}
                        content={item.content}
                        prompt={item.prompt}
                        showLikeButton={showLikeButton}
                        audioBinary={item.audio_binary}
                        showLoudspeaker={showLoudspeaker}
                        showLog={showLog}
                      ></AssistantGroupButton>
                    )}
                    {!isShare && (
                      <AssistantGroupButton
                        messageId={item.id}
                        content={item.content}
                        prompt={item.prompt}
                        showLikeButton={showLikeButton}
                        audioBinary={item.audio_binary}
                        showLoudspeaker={showLoudspeaker}
                        showLog={showLog}
                      ></AssistantGroupButton>
                    )}
                  </>
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
              </div>
            </div>

            {isAssistant &&
              currentEventListWithoutMessageById &&
              showThinking && (
                <div className="mt-4 mb-4">
                  <WorkFlowTimeline
                    currentEventListWithoutMessage={currentEventListWithoutMessageById(
                      item.id,
                    )}
                    isShare={isShare}
                    currentMessageId={item.id}
                    canvasId={conversationId}
                    sendLoading={loading}
                  />
                </div>
              )}
            <div
              className={cn({
                [theme === 'dark'
                  ? styles.messageTextDark
                  : styles.messageText]: isAssistant,
                [styles.messageUserText]: !isAssistant,
                'bg-bg-card': !isAssistant,
              })}
            >
              {item.data ? (
                children
              ) : sendLoading && isEmpty(item.content) ? (
                <>{!isShare && 'running...'}</>
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

            {isUser && (
              <UploadedMessageFiles files={item.files}></UploadedMessageFiles>
            )}
            {isAssistant && item.attachment && item.attachment.doc_id && (
              <div className="w-full flex items-center justify-end">
                <Button
                  variant="link"
                  className="p-1 m-0 h-auto text-text-sub-title-invert"
                  onClick={async () => {
                    if (item.attachment?.doc_id) {
                      try {
                        const response = await downloadFile({
                          docId: item.attachment.doc_id,
                          ext: item.attachment.format,
                        });
                        const blob = new Blob([response.data], {
                          type: response.data.type,
                        });
                        downloadFileFromBlob(blob, item.attachment.file_name);
                      } catch (error) {
                        console.error('Download failed:', error);
                      }
                    }
                  }}
                >
                  <Download size={16} />
                </Button>
              </div>
            )}
          </section>
        </div>
      </section>
    </div>
  );
}

export default memo(MessageItem);
