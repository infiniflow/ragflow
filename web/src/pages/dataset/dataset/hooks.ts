import { useSetModalState } from '@/hooks/common-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { pickByBackend, useIsGoBackend } from '@/utils/backend-variant';
import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useParams } from 'react-router';
import { listDataPipelineLogDocument } from '@/services/knowledge-service';
import { ILogInfo } from '../process-log-modal';
import { RunningStatus } from './constant';
import { useFetchDocumentsByIds } from '@/hooks/use-document-request';
import { isDocumentQueued } from './utils';
import { IFileLogList } from '../dataset-overview/interface';

const PollIntervalMs = 5000;

export const DocumentLogKeys = {
  queued: (datasetId: string | undefined, documentId: string | undefined) =>
    ['queuedDocumentLog', datasetId, documentId] as const,
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
  const queued =
    isGoBackend && !!sourceDoc && isDocumentQueued(sourceDoc);

  // The Go backend reports a queued document via ingestion_status while the
  // legacy document.progress_msg stays empty until the worker starts. Fall
  // back to the early pipeline-operation-log row the Go API writes at task
  // creation, so the queued modal shows "Task is queued..." instead of "-".
  // Python never sets ingestion_status, so this query stays disabled there.
  const { data: queuedLog } = useQuery<IFileLogList>({
    queryKey: DocumentLogKeys.queued(datasetId, sourceDoc?.id),
    enabled: visible && queued && !!datasetId && !!sourceDoc?.id,
    refetchInterval: PollIntervalMs,
    queryFn: async () => {
      const { data: res = {} } = await listDataPipelineLogDocument(
        datasetId || '',
        {
          page: 1,
          page_size: 10,
          keywords: sourceDoc?.name,
          log_type: 'file',
        },
      );
      return (res.data || { logs: [], total: 0 }) as IFileLogList;
    },
  });
  const queuedProgressMsg = useMemo(() => {
    const logs = queuedLog?.logs ?? [];
    const match =
      logs.find((item) => item.document_id === sourceDoc?.id) ?? logs[0];
    return match?.progress_msg;
  }, [queuedLog, sourceDoc?.id]);

  const logInfo = useMemo(() => {
    const source = sourceDoc;
    let log: ILogInfo = {
      taskId: source?.id,
      fileName: source?.name || '-',
      details: queuedProgressMsg || source?.progress_msg || '-',
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
        status: pickByBackend({
          // The Go backend reports a queued document via ingestion_status
          // while the legacy run field stays UNSTART; surface it as QUEUED.
          go: isDocumentQueued(source)
            ? RunningStatus.QUEUED
            : (source.run as RunningStatus),
          python: source.run as RunningStatus,
        }),
        details: queuedProgressMsg || source.progress_msg,
      };
    }
    return log;
  }, [sourceDoc, queuedProgressMsg]);
  const showLog = useCallback(
    (data: IDocumentInfo) => {
      setRecord(data);
      showModal();
    },
    [showModal],
  );
  return { showLog, hideLog: hideModal, logVisible: visible, logInfo };
};
