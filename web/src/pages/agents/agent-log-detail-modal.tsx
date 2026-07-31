import MessageItem from '@/components/next-message-item';
import { Modal } from '@/components/ui/modal/modal';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { IAgentLogMessage } from '@/interfaces/database/agent';
import {
  Docagg,
  IMessage,
  IReferenceChunk,
  IReferenceObject,
  Message,
} from '@/interfaces/database/chat';
import { buildMessageUuidWithRole } from '@/utils/chat';
import React, { useMemo } from 'react';

interface CustomModalProps {
  isOpen: boolean;
  onClose: () => void;
  message: IAgentLogMessage[];
  reference: IReferenceObject;
}

interface IAgentLogReferenceChunk {
  citation_id?: string;
  content?: string | null;
  document_id: string;
  document_name: string;
  dataset_id: string;
  id: string;
  image_id: string;
  positions?: number[] | number[][];
  similarity?: number;
  vector_similarity?: number;
  term_similarity?: number;
  doc_type?: string;
  document_metadata?: Record<string, any>;
  url?: string;
}

// Runtime agent-log messages carry the reference chunks array directly,
// even though IAgentLogMessage does not declare it.
type AgentLogMessageWithReference = IAgentLogMessage & {
  reference?: IAgentLogReferenceChunk[];
};

const buildReferenceObject = (
  chunks: IAgentLogReferenceChunk[] | undefined | null,
): IReferenceObject => {
  if (!chunks || !Array.isArray(chunks) || chunks.length === 0) {
    return { chunks: {}, doc_aggs: {} };
  }

  const chunkMap: Record<string, IReferenceChunk> = {};
  const docAggMap: Record<string, Docagg> = {};

  chunks.forEach((chunk, index) => {
    const key = chunk.citation_id ?? String(index);

    chunkMap[key] = {
      id: chunk.id,
      content: (chunk.content ?? null) as IReferenceChunk['content'],
      document_id: chunk.document_id,
      document_name: chunk.document_name,
      dataset_id: chunk.dataset_id,
      image_id: chunk.image_id,
      similarity: chunk.similarity ?? 0,
      vector_similarity: chunk.vector_similarity ?? 0,
      term_similarity: chunk.term_similarity ?? 0,
      positions: (chunk.positions ?? []) as IReferenceChunk['positions'],
      doc_type: chunk.doc_type,
      document_metadata: chunk.document_metadata,
    };

    if (!docAggMap[chunk.document_id]) {
      docAggMap[chunk.document_id] = {
        count: 0,
        doc_id: chunk.document_id,
        doc_name: chunk.document_name,
        url: chunk.url,
      };
    }
    docAggMap[chunk.document_id].count++;
  });

  return { chunks: chunkMap, doc_aggs: docAggMap };
};

export const AgentLogDetailModal: React.FC<CustomModalProps> = ({
  isOpen,
  onClose,
  message: derivedMessages,
}) => {
  const { data: userInfo } = useFetchUserInfo();
  const { data: canvasInfo } = useFetchAgent();

  const shortMessage = useMemo(() => {
    if (derivedMessages?.length) {
      const content = derivedMessages[0]?.content || '';

      const chineseCharCount = (content.match(/[\u4e00-\u9fa5]/g) || []).length;
      const totalLength = content.length;

      if (chineseCharCount > 0) {
        if (totalLength > 15) {
          return content.substring(0, 15) + '...';
        }
      } else {
        if (totalLength > 30) {
          return content.substring(0, 30) + '...';
        }
      }
      return content;
    } else {
      return '';
    }
  }, [derivedMessages]);

  return (
    <Modal
      open={isOpen}
      onCancel={onClose}
      showfooter={false}
      footer={null}
      title={shortMessage || ''}
      className="!w-[900px]"
    >
      <div className="flex items-start mb-4 flex-col gap-4 justify-start">
        <div className="w-full">
          {derivedMessages?.map((message, i) => {
            const msg = message as AgentLogMessageWithReference;
            return (
              <MessageItem
                key={buildMessageUuidWithRole(
                  message as Partial<Message | IMessage>,
                )}
                nickname={userInfo.nickname}
                avatar={userInfo.avatar}
                avatarDialog={canvasInfo.avatar}
                item={message as IMessage}
                reference={buildReferenceObject(msg.reference)}
                index={i}
                showLikeButton={false}
                showLog={false}
              ></MessageItem>
            );
          })}
        </div>
      </div>
    </Modal>
  );
};
