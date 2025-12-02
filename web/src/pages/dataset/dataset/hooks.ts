import { useSetModalState } from '@/hooks/common-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useCallback, useMemo, useState } from 'react';
import { ILogInfo } from '../process-log-modal';
import { RunningStatus } from './constant';

export const useShowLog = (documents: IDocumentInfo[]) => {
  const { showModal, hideModal, visible } = useSetModalState();
  const [record, setRecord] = useState<IDocumentInfo>();
  const logInfo = useMemo(() => {
    const findRecord = documents.find(
      (item: IDocumentInfo) => item.id === record?.id,
    );
    let log: ILogInfo = {
      taskId: record?.id,
      fileName: record?.name || '-',
      details: record?.progress_msg || '-',
    };
    if (findRecord) {
      log = {
        fileType: findRecord?.suffix,
        uploadedBy: findRecord?.nickname,
        fileName: findRecord?.name,
        uploadDate: formatDate(findRecord.create_date),
        fileSize: formatBytes(findRecord.size || 0),
        processBeginAt: formatDate(findRecord.process_begin_at),
        chunkNumber: findRecord.chunk_num,
        duration: formatSecondsToHumanReadable(
          findRecord.process_duration || 0,
        ),
        status: findRecord.run as RunningStatus,
        details: findRecord.progress_msg,
      };
    }
    return log;
  }, [record, documents]);
  const showLog = useCallback(
    (data: IDocumentInfo) => {
      setRecord(data);
      showModal();
    },
    [showModal],
  );
  return { showLog, hideLog: hideModal, logVisible: visible, logInfo };
};
