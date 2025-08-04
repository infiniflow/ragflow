import { MessageType } from '@/constants/chat';
import { useGetFileIcon } from '@/pages/chat/hooks';

import { useSendAgentMessage } from './use-send-agent-message';

import { FileUploadProps } from '@/components/file-upload';
import { NextMessageInput } from '@/components/message-input/next';
import MessageItem from '@/components/next-message-item';
import PdfDrawer from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import {
  useFetchAgent,
  useUploadCanvasFileWithProgress,
} from '@/hooks/use-agent-request';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { memo, useCallback } from 'react';
import { useParams } from 'umi';
import DebugContent from '../debug-content';
import { useAwaitCompentData } from '../hooks/use-chat-logic';

function AgentChatBox() {
  const {
    value,
    scrollRef,
    messageContainerRef,
    sendLoading,
    derivedMessages,
    handleInputChange,
    handlePressEnter,
    stopOutputMessage,
    sendFormMessage,
    findReferenceByMessageId,
    appendUploadResponseList,
  } = useSendAgentMessage();

  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();
  useGetFileIcon();
  const { data: userInfo } = useFetchUserInfo();
  const { data: canvasInfo } = useFetchAgent();
  const { id: canvasId } = useParams();
  const { uploadCanvasFile, loading } = useUploadCanvasFileWithProgress();

  const { buildInputList, handleOk, isWaitting } = useAwaitCompentData({
    derivedMessages,
    sendFormMessage,
    canvasId: canvasId as string,
  });

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
      <section className="flex flex-1 flex-col px-5 h-[90vh]">
        <div className="flex-1 overflow-auto" ref={messageContainerRef}>
          <div>
            {/* <Spin spinning={sendLoading}> */}
            {derivedMessages?.map((message, i) => {
              return (
                <MessageItem
                  loading={
                    message.role === MessageType.Assistant &&
                    sendLoading &&
                    derivedMessages.length - 1 === i
                  }
                  key={buildMessageUuidWithRole(message)}
                  nickname={userInfo.nickname}
                  avatar={userInfo.avatar}
                  avatarDialog={canvasInfo.avatar}
                  item={message}
                  reference={findReferenceByMessageId(message.id)}
                  clickDocumentButton={clickDocumentButton}
                  index={i}
                  showLikeButton={false}
                  sendLoading={sendLoading}
                >
                  {message.role === MessageType.Assistant &&
                    derivedMessages.length - 1 === i && (
                      <DebugContent
                        parameters={buildInputList(message)}
                        message={message}
                        ok={handleOk(message)}
                        isNext={false}
                        btnText={'Submit'}
                      ></DebugContent>
                    )}
                  {message.role === MessageType.Assistant &&
                    derivedMessages.length - 1 !== i && (
                      <div>
                        <div>{message?.data?.tips}</div>

                        <div>
                          {buildInputList(message)?.map((item) => item.value)}
                        </div>
                      </div>
                    )}
                </MessageItem>
              );
            })}
            {/* </Spin> */}
          </div>
          <div ref={scrollRef} />
        </div>
        <NextMessageInput
          value={value}
          sendLoading={sendLoading}
          disabled={isWaitting}
          sendDisabled={sendLoading || isWaitting}
          isUploading={loading || isWaitting}
          onPressEnter={handlePressEnter}
          onInputChange={handleInputChange}
          stopOutputMessage={stopOutputMessage}
          onUpload={handleUploadFile}
          conversationId=""
        />
      </section>
      <PdfDrawer
        visible={visible}
        hideModal={hideModal}
        documentId={documentId}
        chunk={selectedChunk}
      ></PdfDrawer>
    </>
  );
}

export default memo(AgentChatBox);
