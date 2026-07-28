import { useSetModalState } from '@/hooks/common-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useCallback, useMemo, useState } from 'react';
import { ILogInfo } from '../process-log-modal';
import { RunningStatus } from './constant';
import { useFetchDocumentsByIds } from '@/hooks/use-document-request';

const PollIntervalMs = 5000;

export const useShowLog = (documents: IDocumentInfo[]) => {
  const { showModal, hideModal, visible } = useSetModalState();
  const [record, setRecord] = useState<IDocumentInfo>();

  // When the modal is visible, poll the document directly by ID so progress_msg
  // updates (e.g. "Indexing done") are captured even if the parent list no longer
  // polls (isLoop became false before the final message) or the record fell off
  // the current paginated page.
  const { documents: liveDocs } = useFetchDocumentsByIds(
    record?.id ? [record.id] : [],
    { enabled: visible, refetchInterval: PollIntervalMs },
  );
  const liveDoc = liveDocs?.[0];

  const logInfo = useMemo(() => {
    const source = liveDoc ?? documents.find(
      (item: IDocumentInfo) => item.id === record?.id,
    ) ?? record;
    let log: ILogInfo = {
      taskId: source?.id,
      fileName: source?.name || '-',
      details: source?.progress_msg || '-',
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
        duration: formatSecondsToHumanReadable(
          source.process_duration || 0,
        ),
        status: source.run as RunningStatus,
        details: source.progress_msg,
      };
    }
    return log;
  }, [record, documents, liveDoc]);
  const showLog = useCallback(
    (data: IDocumentInfo) => {
      setRecord(data);
      showModal();
    },
    [showModal],
  );
  return { showLog, hideLog: hideModal, logVisible: visible, logInfo };
};
