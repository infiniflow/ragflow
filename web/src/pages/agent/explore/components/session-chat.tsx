import { FileUploadProps } from '@/components/file-upload';
import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/next-message-item';
import PdfSheet from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { MessageType } from '@/constants/chat';
import { useUploadCanvasFileWithProgress } from '@/hooks/use-agent-request';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useSendSessionMessage } from '../hooks/use-send-session-message';

interface SessionChatProps {
  sessionId?: string;
}

export function SessionChat({ sessionId }: SessionChatProps) {
  const { t } = useTranslation();
  const { data: userInfo } = useFetchUserInfo();

  // Use custom hook for chat logic
  const {
    value,
    derivedMessages,
    scrollRef,
    messageContainerRef,
    sendLoading,
    sessionLoading,
    handleInputChange,
    handlePressEnter,
    stopOutputMessage,
    canvasInfo,
    findReferenceByMessageId,
    appendUploadResponseList,
    removeFile,
  } = useSendSessionMessage();

  // PDF drawer for reference preview
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  // File upload
  const { uploadCanvasFile, loading: isUploading } =
    useUploadCanvasFileWithProgress();

  const handleUploadFile: NonNullable<FileUploadProps['onUpload']> =
    useCallback(
      async (files, options) => {
        const ret = await uploadCanvasFile({ files, options });
        appendUploadResponseList(ret.data, files);
      },
      [appendUploadResponseList, uploadCanvasFile],
    );

  return (
    <>
      <section className="flex flex-col h-full">
        {!sessionId && (
          <div className="flex-1 flex items-center justify-center text-text-secondary">
            {t('explore.noSessionSelected')}
          </div>
        )}

        {sessionId && (
          <div
            ref={messageContainerRef}
            className="flex-1 overflow-auto min-h-0 p-5"
          >
            {sessionLoading ? (
              <div className="flex items-center justify-center h-full">
                Loading...
              </div>
            ) : derivedMessages.length === 0 ? (
              <div className="flex items-center justify-center h-full text-text-secondary">
                No messages in this session
              </div>
            ) : (
              <div className="w-full pr-5">
                {derivedMessages.map((message, i) => (
                  <MessageItem
                    loading={
                      message.role === MessageType.Assistant &&
                      sendLoading &&
                      derivedMessages.length - 1 === i
                    }
                    key={buildMessageUuidWithRole(message)}
                    item={message}
                    nickname={userInfo.nickname}
                    avatar={userInfo.avatar}
                    avatarDialog={canvasInfo?.avatar || ''}
                    reference={findReferenceByMessageId(message.id)}
                    clickDocumentButton={clickDocumentButton}
                    index={i}
                    showLikeButton={false}
                    sendLoading={sendLoading}
                  />
                ))}
              </div>
            )}
            <div ref={scrollRef} />
          </div>
        )}
        <section className="p-4">
          <NextMessageInput
            value={value}
            sendLoading={sendLoading}
            disabled={false}
            sendDisabled={sendLoading}
            isUploading={isUploading}
            onPressEnter={handlePressEnter}
            onInputChange={handleInputChange}
            stopOutputMessage={stopOutputMessage}
            onUpload={handleUploadFile}
            removeFile={removeFile}
            conversationId=""
          />
        </section>
      </section>

      {/* PDF Preview */}
      {visible && (
        <PdfSheet
          visible={visible}
          hideModal={hideModal}
          documentId={documentId}
          chunk={selectedChunk}
        />
      )}
    </>
  );
}
