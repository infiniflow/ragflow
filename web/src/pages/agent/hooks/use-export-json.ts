import { useToast } from '@/components/hooks/use-toast';
import { FileMimeType, Platform } from '@/constants/common';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { IGraph } from '@/interfaces/database/flow';
import { downloadJsonFile } from '@/utils/file-util';
import { message } from 'antd';
import isEmpty from 'lodash/isEmpty';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useBuildDslData } from './use-build-dsl';
import { useSetGraphInfo } from './use-set-graph';

export const useHandleExportOrImportJsonFile = () => {
  const { buildDslData } = useBuildDslData();
  const {
    visible: fileUploadVisible,
    hideModal: hideFileUploadModal,
    showModal: showFileUploadModal,
  } = useSetModalState();
  const setGraphInfo = useSetGraphInfo();
  const { data } = useFetchAgent();
  const { t } = useTranslation();
  const { toast } = useToast();

  const onFileUploadOk = useCallback(
    async ({
      fileList,
      platform,
    }: {
      fileList: File[];
      platform: Platform;
    }) => {
      console.log('🚀 ~ useHandleExportOrImportJsonFile ~ platform:', platform);
      if (fileList.length > 0) {
        const file = fileList[0];
        if (file.type !== FileMimeType.Json) {
          toast({ title: t('flow.jsonUploadTypeErrorMessage') });
          return;
        }

        const graphStr = await file.text();
        const errorMessage = t('flow.jsonUploadContentErrorMessage');
        try {
          const graph = JSON.parse(graphStr);
          if (graphStr && !isEmpty(graph) && Array.isArray(graph?.nodes)) {
            setGraphInfo(graph ?? ({} as IGraph));
            hideFileUploadModal();
          } else {
            message.error(errorMessage);
          }
        } catch (error) {
          message.error(errorMessage);
        }
      }
    },
    [hideFileUploadModal, setGraphInfo, t, toast],
  );

  const handleExportJson = useCallback(() => {
    downloadJsonFile(buildDslData().graph, `${data.title}.json`);
  }, [buildDslData, data.title]);

  return {
    fileUploadVisible,
    handleExportJson,
    handleImportJson: showFileUploadModal,
    hideFileUploadModal,
    onFileUploadOk,
  };
};
