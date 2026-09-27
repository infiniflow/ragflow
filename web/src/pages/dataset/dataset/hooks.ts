import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchDocumentsByIds } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { useIsGoBackend } from '@/utils/backend-variant';
import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useQuery } from '@tanstack/react-query';
import { RunningStatus } from '@/constants/knowledge';
import { useCallback, useMemo, useState } from 'react';
import { useParams } from 'react-router';
import { listDataPipelineLogDocument } from '@/services/knowledge-service';
import { ILogInfo } from '../process-log-modal';
import { getDocumentProgressMessage, getDocumentRunningStatus } from './utils';
import type { IFileLogList } from '../dataset-overview/interface';
import { useIngestionMessages } from '../ingestion-message-hooks';

const PollIntervalMs = 5000;

export const DocumentLogKeys = {
  queued: (datasetId: string | undefined, documentId: string | undefined) =>
    ['documentLog', datasetId, documentId] as const,
};

export const useShowLog = (documents: IDocumentInfo[]) => {
  const { showModal, hideModal, visible } = useSetModalState();
  const [record, setRecord] = useState<IDocumentInfo>();
  const { id: routeId } = useParams();
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const datasetId = knowledgeId || routeId;
  const isGoBackend = useIsGoBackend();

  const isTerminal = (doc?: IDocumentInfo) => {
    const status = doc && getDocumentRunningStatus(doc);
    return (
      status === RunningStatus.DONE ||
      status === RunningStatus.FAIL ||
      status === RunningStatus.CANCEL
    );
  };

  // When the modal is visible, poll the document directly by ID so progress_msg
  // updates (e.g. "Indexing done") are captured even if the parent list no longer
  // polls (isLoop became false before the final message) or the record fell off
  // the current paginated page. Stop polling once the run reaches a terminal
  // status, otherwise the modal keeps requesting forever.
  const { documents: liveDocs } = useFetchDocumentsByIds(
    record?.id ? [record.id] : [],
    {
      enabled: visible,
      refetchInterval: (query) => {
        const doc =
          query.state.data?.docs[0] ??
          documents.find((item: IDocumentInfo) => item.id === record?.id) ??
          record;
        return isTerminal(doc) ? false : PollIntervalMs;
      },
    },
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
    refetchInterval: isTerminal(sourceDoc) ? false : PollIntervalMs,
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
          orderby: 'run_count',
          desc: true,
          page_size: 1,
        },
      );
      return (res.data || { logs: [], total: 0 }) as IFileLogList;
    },
  });
  const logID = documentLog?.logs[0]?.id;
  const {
    data: messages,
    fetchPreviousPage,
    hasPreviousPage,
    isFetchingPreviousPage,
  } = useIngestionMessages(datasetId, logID, visible);
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
        ? messages?.items.length
          ? ''
          : getDocumentProgressMessage({
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
        details: messages?.items.length
          ? ''
          : getDocumentProgressMessage({
              ...source,
              latest_ingestion_event:
                latestEvent ?? source.latest_ingestion_event,
            }),
        events: messages?.items,
        loadPreviousEvents: hasPreviousPage
          ? () => fetchPreviousPage()
          : undefined,
        hasPreviousEvents: hasPreviousPage,
        isLoadingPreviousEvents: isFetchingPreviousPage,
      };
    }
    return log;
  }, [
    sourceDoc,
    latestEvent,
    messages,
    fetchPreviousPage,
    hasPreviousPage,
    isFetchingPreviousPage,
  ]);
  const showLog = useCallback(
    (data: IDocumentInfo) => {
      setRecord(data);
      showModal();
    },
    [showModal],
  );
  return { showLog, hideLog: hideModal, logVisible: visible, logInfo };
};
