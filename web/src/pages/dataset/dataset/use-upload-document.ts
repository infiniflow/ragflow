import { UploadFormSchemaType } from '@/components/file-upload-dialog';
import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useRunDocument,
  useUploadDocument,
} from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { getUnSupportedFilesCount } from '@/utils/document-util';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { buildUploadGapModalContent } from './parser-gap-content';
import { useParserGapValidation } from './use-parser-gap-validation';
import { hasUnsupportedTypeGap } from './utils';

export const useHandleUploadDocument = () => {
  const { t } = useTranslation();
  const {
    visible: documentUploadVisible,
    hideModal: hideDocumentUploadModal,
    showModal: showDocumentUploadModal,
  } = useSetModalState();
  const { uploadDocument, loading } = useUploadDocument();
  const { runDocumentByIds } = useRunDocument();
  const { findParseGaps, findDocumentParseGaps } = useParserGapValidation();

  const proceedUpload = useCallback(
    async ({
      fileList,
      parseOnCreation,
      tableColumnMode,
      tableColumnRoles,
    }: UploadFormSchemaType) => {
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

      // Auto-parse is decided from each uploaded document's own parser_config —
      // the config the backend resolved for it (an undeclared family is
      // reseeded with the pipeline that owns it, or with general). This is the
      // same validation the parse button applies, so files whose required model
      // is missing are uploaded but not auto-parsed.
      if (
        (isSuccess || isPartialSuccess) &&
        parseOnCreation &&
        ret.data?.length > 0
      ) {
        const runnableDocuments = (ret.data as IDocumentInfo[]).filter(
          (doc) => findDocumentParseGaps([doc]).length === 0,
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
    [
      uploadDocument,
      runDocumentByIds,
      findDocumentParseGaps,
      hideDocumentUploadModal,
    ],
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
      const gaps = findParseGaps(names);
      if (gaps.length > 0) {
        Modal.warning({
          title: t(
            hasUnsupportedTypeGap(gaps)
              ? 'knowledgeDetails.uploadUnsupportedTypesTitle'
              : 'knowledgeDetails.uploadMissingModelsTitle',
          ),
          content: buildUploadGapModalContent(t, gaps, {
            missingModel: 'knowledgeDetails.addModelAfterUploadHint',
            autoSwitch: 'knowledgeDetails.reselectParserAfterUploadHint',
            manualSwitch: 'knowledgeDetails.manualSwitchAfterUploadHint',
            mixedSwitch: 'knowledgeDetails.mixedSwitchAfterUploadHint',
          }),
          okText: t('knowledgeDetails.continueUpload'),
          cancelText: t('common.cancel'),
          closable: false,
          onOk: () => {
            proceedUpload(values);
          },
        });
        return;
      }

      return proceedUpload(values);
    },
    [findParseGaps, proceedUpload, t],
  );

  return {
    documentUploadLoading: loading,
    onDocumentUploadOk,
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
  };
};
