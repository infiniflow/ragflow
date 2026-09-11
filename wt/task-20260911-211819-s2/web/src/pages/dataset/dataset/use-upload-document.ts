import { UploadFormSchemaType } from '@/components/file-upload-dialog';
import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useRunDocument,
  useUploadDocument,
} from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { FileType } from '@/pages/agent/constant/pipeline';
import { getExtension, getUnSupportedFilesCount } from '@/utils/document-util';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { buildMissingModelModalContent } from './parser-model-gap-content';
import { useParserModelValidation } from './use-parser-model-validation';
import { getFileTypeByExtension } from './utils';

export const useHandleUploadDocument = () => {
  const { t } = useTranslation();
  const {
    visible: documentUploadVisible,
    hideModal: hideDocumentUploadModal,
    showModal: showDocumentUploadModal,
  } = useSetModalState();
  const { uploadDocument, loading } = useUploadDocument();
  const { runDocumentByIds } = useRunDocument();
  const { findFilesMissingModels, goToDatasetConfiguration } =
    useParserModelValidation();

  const proceedUpload = useCallback(
    async (
      {
        fileList,
        parseOnCreation,
        tableColumnMode,
        tableColumnRoles,
      }: UploadFormSchemaType,
      failingFileTypes: Set<FileType>,
    ) => {
      // Build parser_config if column roles are configured
      let parserConfig: Record<string, any> | undefined;
      if (
        tableColumnMode === 'manual' &&
        tableColumnRoles &&
        Object.keys(tableColumnRoles).length > 0
      ) {
        parserConfig = {
          table_column_mode: 'manual',
          table_column_roles: tableColumnRoles,
        };
      }

      const ret = await uploadDocument(fileList as File[], parserConfig);

      // Check for success (code === 0) or partial success (code === 500 with some files)
      const isSuccess = ret?.code === 0;
      const isPartialSuccess = ret?.code === 500 && ret?.message;

      if (!isSuccess && !isPartialSuccess) {
        return;
      }

      // Trigger parsing for both full and partial success when parseOnCreation
      // is enabled; files whose required model is missing are uploaded but not
      // auto-parsed.
      if (
        (isSuccess || isPartialSuccess) &&
        parseOnCreation &&
        ret.data?.length > 0
      ) {
        const runnableDocuments = (ret.data as IDocumentInfo[]).filter(
          (doc) => {
            const fileType = getFileTypeByExtension(getExtension(doc.name));
            return !fileType || !failingFileTypes.has(fileType);
          },
        );
        if (runnableDocuments.length > 0) {
          runDocumentByIds({
            documentIds: runnableDocuments.map((x) => x.id),
            run: 1,
          });
        }
      }

      if (isSuccess) {
        hideDocumentUploadModal();
        return 0;
      }

      // For partial success (code 500), check if any files were uploaded
      const count = getUnSupportedFilesCount(ret?.message);
      if (count !== fileList.length) {
        hideDocumentUploadModal();
        return 0;
      }

      return ret?.code;
    },
    [uploadDocument, runDocumentByIds, hideDocumentUploadModal],
  );

  const onDocumentUploadOk = useCallback(
    async (values: UploadFormSchemaType) => {
      const { fileList } = values;
      if (fileList.length === 0) {
        return;
      }

      const names = fileList.map((file) =>
        file instanceof File ? file.name : file.file.name,
      );
      const gaps = findFilesMissingModels(names);
      if (gaps.length > 0) {
        const failingFileTypes = new Set(gaps.map((gap) => gap.fileType));
        Modal.warning({
          title: t('knowledgeDetails.uploadMissingModelsTitle'),
          content: buildMissingModelModalContent(
            t,
            gaps,
            'knowledgeDetails.configureInDatasetSettingHint',
          ),
          okText: t('knowledgeDetails.continueUpload'),
          cancelText: t('knowledgeDetails.goToConfiguration'),
          closable: false,
          onOk: () => {
            proceedUpload(values, failingFileTypes);
          },
          onCancel: () => {
            hideDocumentUploadModal();
            goToDatasetConfiguration();
          },
        });
        return;
      }

      return proceedUpload(values, new Set());
    },
    [
      findFilesMissingModels,
      goToDatasetConfiguration,
      hideDocumentUploadModal,
      proceedUpload,
      t,
    ],
  );

  return {
    documentUploadLoading: loading,
    onDocumentUploadOk,
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
  };
};
