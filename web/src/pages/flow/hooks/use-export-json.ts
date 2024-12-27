import { FileMimeType } from '@/constants/common';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { IGraph } from '@/interfaces/database/flow';
import { downloadJsonFile } from '@/utils/file-util';
import { message, UploadFile } from 'antd';
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
  const { data } = useFetchFlow();
  const { t } = useTranslation();

  const onFileUploadOk = useCallback(
    async (fileList: UploadFile[]) => {
      if (fileList.length > 0) {
        const file: File = fileList[0] as unknown as File;
        if (file.type !== FileMimeType.Json) {
          message.error(t('flow.jsonUploadTypeErrorMessage'));
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
    [hideFileUploadModal, setGraphInfo, t],
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
