import { useSetModalState } from '@/hooks/common-hooks';
import { useNextWebCrawl } from '@/hooks/document-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { IDocumentInfo } from '@/interfaces/database/document';
import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { formatBytes } from '@/utils/file-util';
import { useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'umi';
import { ILogInfo } from '../process-log-modal';
import { RunningStatus } from './constant';

export const useNavigateToOtherPage = () => {
  const navigate = useNavigate();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const linkToUploadPage = useCallback(() => {
    navigate(`/knowledge/dataset/upload?id=${knowledgeId}`);
  }, [navigate, knowledgeId]);

  const toChunk = useCallback((id: string) => {}, []);

  return { linkToUploadPage, toChunk };
};

export const useGetRowSelection = () => {
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);

  const rowSelection = {
    selectedRowKeys,
    onChange: (newSelectedRowKeys: React.Key[]) => {
      setSelectedRowKeys(newSelectedRowKeys);
    },
  };

  return rowSelection;
};

export const useHandleWebCrawl = () => {
  const {
    visible: webCrawlUploadVisible,
    hideModal: hideWebCrawlUploadModal,
    showModal: showWebCrawlUploadModal,
  } = useSetModalState();
  const { webCrawl, loading } = useNextWebCrawl();

  const onWebCrawlUploadOk = useCallback(
    async (name: string, url: string) => {
      const ret = await webCrawl({ name, url });
      if (ret === 0) {
        hideWebCrawlUploadModal();
        return 0;
      }
      return -1;
    },
    [webCrawl, hideWebCrawlUploadModal],
  );

  return {
    webCrawlUploadLoading: loading,
    onWebCrawlUploadOk,
    webCrawlUploadVisible,
    hideWebCrawlUploadModal,
    showWebCrawlUploadModal,
  };
};

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
