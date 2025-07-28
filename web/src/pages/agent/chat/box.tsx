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
import { Message } from '@/interfaces/database/chat';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { get } from 'lodash';
import { useCallback } from 'react';
import { useParams } from 'umi';
import DebugContent from '../debug-content';
import { BeginQuery } from '../interface';
import { buildBeginQueryWithObject } from '../utils';

const AgentChatBox = () => {
  const {
    value,
    ref,
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

  const getInputs = useCallback((message: Message) => {
    return get(message, 'data.inputs', {}) as Record<string, BeginQuery>;
  }, []);

  const buildInputList = useCallback(
    (message: Message) => {
      return Object.entries(getInputs(message)).map(([key, val]) => {
        return {
          ...val,
          key,
        };
      });
    },
    [getInputs],
  );

  const handleOk = useCallback(
    (message: Message) => (values: BeginQuery[]) => {
      const inputs = getInputs(message);
      const nextInputs = buildBeginQueryWithObject(inputs, values);
      sendFormMessage({
        inputs: nextInputs,
        id: canvasId,
      });
    },
    [canvasId, getInputs, sendFormMessage],
  );

  const handleUploadFile: NonNullable<FileUploadProps['onUpload']> =
    useCallback(
      async (files, options) => {
        const ret = await uploadCanvasFile({ files, options });
        appendUploadResponseList(ret.data);
      },
      [appendUploadResponseList, uploadCanvasFile],
    );

  return (
    <>
      <section className="flex flex-1 flex-col px-5 h-[90vh]">
        <div className="flex-1 overflow-auto">
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
                  <DebugContent
                    parameters={buildInputList(message)}
                    ok={handleOk(message)}
                    isNext={false}
                    btnText={'Submit'}
                  ></DebugContent>
                </MessageItem>
              );
            })}
            {/* </Spin> */}
          </div>
          <div ref={ref} />
        </div>
        <NextMessageInput
          value={value}
          sendLoading={sendLoading}
          disabled={false}
          sendDisabled={sendLoading}
          isUploading={loading}
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
};

export default AgentChatBox;
