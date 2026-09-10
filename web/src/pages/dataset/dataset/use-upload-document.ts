import { UploadFormSchemaType } from '@/components/file-upload-dialog';
import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useRunDocument,
  useUploadDocument,
} from '@/hooks/use-document-request';
import { FileType } from '@/constants/file';
import { IDocumentInfo } from '@/interfaces/database/document';
import { getExtension, getUnSupportedFilesCount } from '@/utils/document-util';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { buildParserGapModalContent } from './parser-gap-content';
import { useParserGapValidation } from './use-parser-gap-validation';
import { getFileTypeByExtension, hasUnsupportedTypeGap } from './utils';

export const useHandleUploadDocument = () => {
  const { t } = useTranslation();
  const {
    visible: documentUploadVisible,
    hideModal: hideDocumentUploadModal,
    showModal: showDocumentUploadModal,
  } = useSetModalState();
  const { uploadDocument, loading } = useUploadDocument();
  const { runDocumentByIds } = useRunDocument();
  const { findParseGaps } = useParserGapValidation();

  const proceedUpload = useCallback(
    async (
      {
        fileList,
        parseOnCreation,
        tableColumnMode,
        tableColumnNames,
        tableColumnNamesByFile,
        tableColumnRoles,
      }: UploadFormSchemaType,
      failingFileTypes: Set<FileType>,
    ) => {
      // Only table files carry column settings, and only those files get a
      // per-file entry — never emit names/roles for mixed non-table uploads.
      const tableIndexes = (fileList as any[])
        .map((f: any, i: number) => {
          const file = f instanceof File ? f : f?.file;
          const name =
            file instanceof File
              ? file.name
              : typeof f?.name === 'string'
                ? f.name
                : '';
          return /\.(csv|xlsx?|txt)$/i.test(name) ? i : -1;
        })
        .filter((i: number) => i >= 0);
      // Build parser_config if column settings are configured
      let parserConfig: Record<string, any> | undefined;
      if (tableIndexes.length > 0 && tableColumnMode) {
        parserConfig = {
          table_column_mode: tableColumnMode,
        };
        if (tableColumnNames?.length) {
          parserConfig.table_column_names = tableColumnNames;
        }
        if (Array.isArray(tableColumnNamesByFile)) {
          const byFile = tableIndexes.map(
            (i: number) => tableColumnNamesByFile[i] ?? [],
          );
          if (byFile.some((cols: string[]) => cols.length > 0)) {
            parserConfig.table_column_names_by_file = byFile;
          }
        }
        if (tableColumnMode === 'manual' && tableColumnRoles) {
          parserConfig.table_column_roles = tableColumnRoles;
        }
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
      const gaps = findParseGaps(names);
      if (gaps.length > 0) {
        const failingFileTypes = new Set(gaps.map((gap) => gap.fileType));
        Modal.warning({
          title: t(
            hasUnsupportedTypeGap(gaps)
              ? 'knowledgeDetails.uploadUnsupportedTypesTitle'
              : 'knowledgeDetails.uploadMissingModelsTitle',
          ),
          content: buildParserGapModalContent(t, gaps, {
            missingModel: 'knowledgeDetails.addModelAfterUploadHint',
            unsupportedType: 'knowledgeDetails.reselectParserAfterUploadHint',
          }),
          okText: t('knowledgeDetails.continueUpload'),
          cancelText: t('common.cancel'),
          closable: false,
          onOk: () => {
            proceedUpload(values, failingFileTypes);
          },
        });
        return;
      }

      return proceedUpload(values, new Set());
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
