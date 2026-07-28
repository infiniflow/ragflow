import { useSetModalState } from '@/hooks/common-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useCallback, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ILogInfo } from '../process-log-modal';
import { RunningStatus } from './constant';
import { listDocument } from '@/services/knowledge-service';
import { useParams } from 'react-router';

const PollIntervalMs = 5000;

export const useShowLog = (documents: IDocumentInfo[]) => {
  const { showModal, hideModal, visible } = useSetModalState();
  const [record, setRecord] = useState<IDocumentInfo>();
  const { id: datasetId } = useParams();

  // When the modal is visible, poll the document directly by ID so progress_msg
  // updates (e.g. "Indexing done") are captured even if the parent list no longer
  // polls (isLoop became false before the final message) or the record fell off
  // the current paginated page.
  const { data: liveDoc } = useQuery({
    queryKey: ['fetchDocumentLog', record?.id, visible],
    enabled: visible && !!record?.id && !!datasetId,
    refetchInterval: PollIntervalMs,
    queryFn: async () => {
      const ret = await listDocument(
        { id: datasetId!, page_size: 1, page: 1 },
        { ids: [record!.id] },
      );
      return ret.data?.data?.docs?.[0] as IDocumentInfo | undefined;
    },
  });

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
