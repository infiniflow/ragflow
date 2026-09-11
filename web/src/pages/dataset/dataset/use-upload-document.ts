import { UploadFormSchemaType } from '@/components/file-upload-dialog';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useRunDocument,
  useUploadDocument,
} from '@/hooks/use-document-request';
import { getUnSupportedFilesCount } from '@/utils/document-util';
import { useCallback } from 'react';

export const useHandleUploadDocument = () => {
  const {
    visible: documentUploadVisible,
    hideModal: hideDocumentUploadModal,
    showModal: showDocumentUploadModal,
  } = useSetModalState();
  const { uploadDocument, loading } = useUploadDocument();
  const { runDocumentByIds } = useRunDocument();

  const onDocumentUploadOk = useCallback(
    async ({
      fileList,
      parseOnCreation,
      tableColumnMode,
      tableColumnNames,
      tableColumnNamesByFile,
      tableColumnRoles,
    }: UploadFormSchemaType) => {
      if (fileList.length > 0) {
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

        // Trigger parsing for both full and partial success when parseOnCreation is enabled
        if (
          (isSuccess || isPartialSuccess) &&
          parseOnCreation &&
          ret.data?.length > 0
        ) {
          runDocumentByIds({
            documentIds: ret.data.map((x: any) => x.id),
            run: 1,
          });
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
      }
    },
    [uploadDocument, runDocumentByIds, hideDocumentUploadModal],
  );

  return {
    documentUploadLoading: loading,
    onDocumentUploadOk,
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
  };
};
