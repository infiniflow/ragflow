import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchDocumentsByIds } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IngestionMessagesResponse } from '@/interfaces/database/ingestion';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { useIsGoBackend } from '@/utils/backend-variant';
import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useParams } from 'react-router';
import {
  listDataPipelineLogDocument,
  listIngestionMessages,
} from '@/services/knowledge-service';
import { ILogInfo } from '../process-log-modal';
import { getDocumentProgressMessage, getDocumentRunningStatus } from './utils';
import type { IFileLogList } from '../dataset-overview/interface';

const PollIntervalMs = 5000;

export const DocumentLogKeys = {
  queued: (datasetId: string | undefined, documentId: string | undefined) =>
    ['documentLog', datasetId, documentId] as const,
  messages: (datasetId: string | undefined, logId: string | undefined) =>
    ['ingestionMessages', datasetId, logId] as const,
};

export const useShowLog = (documents: IDocumentInfo[]) => {
  const { showModal, hideModal, visible } = useSetModalState();
  const [record, setRecord] = useState<IDocumentInfo>();
  const { id: routeId } = useParams();
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const datasetId = knowledgeId || routeId;
  const isGoBackend = useIsGoBackend();

  // When the modal is visible, poll the document directly by ID so progress_msg
  // updates (e.g. "Indexing done") are captured even if the parent list no longer
  // polls (isLoop became false before the final message) or the record fell off
  // the current paginated page.
  const { documents: liveDocs } = useFetchDocumentsByIds(
    record?.id ? [record.id] : [],
    { enabled: visible, refetchInterval: PollIntervalMs },
  );
  const liveDoc = liveDocs?.[0];
  const sourceDoc =
    liveDoc ??
    documents.find((item: IDocumentInfo) => item.id === record?.id) ??
    record;
  // A document can have several historical runs. The list endpoint's exact
  // document filter returns the current log identity, which scopes all event
  // reads to the intended run.
  const { data: documentLog } = useQuery<IFileLogList>({
    queryKey: DocumentLogKeys.queued(datasetId, sourceDoc?.id),
    enabled: visible && isGoBackend && !!datasetId && !!sourceDoc?.id,
    refetchInterval: PollIntervalMs,
    queryFn: async () => {
      const { data: res = {} } = await listDataPipelineLogDocument(
        datasetId || '',
        {
          page: 1,
          // Exact match on the document: a name search is fuzzy and can push
          // this document's row off the first page when several documents
          // share a name.
          document_id: sourceDoc?.id,
          log_type: 'file',
          orderby: 'create_time',
          desc: true,
          page_size: 1,
        },
      );
      return (res.data || { logs: [], total: 0 }) as IFileLogList;
    },
  });
  const logID = documentLog?.logs[0]?.id;
  const { data: messages } = useQuery<IngestionMessagesResponse>({
    queryKey: DocumentLogKeys.messages(datasetId, logID),
    enabled: visible && isGoBackend && !!datasetId && !!logID,
    refetchInterval: (query) =>
      query.state.data?.terminal ? false : PollIntervalMs,
    queryFn: async () => {
      const { data: res = {} } = await listIngestionMessages(
        datasetId || '',
        logID || '',
        { limit: 200 },
      );
      return res.data as IngestionMessagesResponse;
    },
  });
  const latestEvent = useMemo(() => {
    const items = messages?.items ?? [];
    return items[items.length - 1];
  }, [messages]);

  const logInfo = useMemo(() => {
    const source = sourceDoc;
    let log: ILogInfo = {
      taskId: source?.id,
      fileName: source?.name || '-',
      details: source
        ? getDocumentProgressMessage({
            ...source,
            latest_ingestion_event:
              latestEvent ?? source.latest_ingestion_event,
          })
        : '-',
    };
    if (source) {
      log = {
        fileType: source?.suffix,
        uploadedBy: source?.nickname,
        fileName: source?.name,
        uploadDate: formatDate(source.create_date),
        fileSize: formatBytes(source.size || 0),
        processBeginAt: formatDate(source.process_begin_at),
        chunkNumber: source.chunk_count,
        duration: formatSecondsToHumanReadable(source.process_duration || 0),
        // Go derives status from ingestion_status (queued included);
        // Python reads the legacy run field.
        status: getDocumentRunningStatus(source),
        details: getDocumentProgressMessage({
          ...source,
          latest_ingestion_event:
            latestEvent ?? source.latest_ingestion_event,
        }),
      };
    }
    return log;
  }, [sourceDoc, latestEvent]);
  const showLog = useCallback(
    (data: IDocumentInfo) => {
      setRecord(data);
      showModal();
    },
    [showModal],
  );
  return { showLog, hideLog: hideModal, logVisible: visible, logInfo };
};
