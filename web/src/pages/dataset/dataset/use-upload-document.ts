import { useSetModalState } from '@/hooks/common-hooks';
import { useUploadNextDocument } from '@/hooks/use-document-request';
import { getUnSupportedFilesCount } from '@/utils/document-util';
import { useCallback } from 'react';

export const useHandleUploadDocument = () => {
  const {
    visible: documentUploadVisible,
    hideModal: hideDocumentUploadModal,
    showModal: showDocumentUploadModal,
  } = useSetModalState();
  const { uploadDocument, loading } = useUploadNextDocument();

  const onDocumentUploadOk = useCallback(
    async (fileList: File[]): Promise<number | undefined> => {
      if (fileList.length > 0) {
        const ret: any = await uploadDocument(fileList);
        if (typeof ret?.message !== 'string') {
          return;
        }
        const count = getUnSupportedFilesCount(ret?.message);
        /// 500 error code indicates that some file types are not supported
        let code = ret?.code;
        if (
          ret?.code === 0 ||
          (ret?.code === 500 && count !== fileList.length) // Some files were not uploaded successfully, but some were uploaded successfully.
        ) {
          code = 0;
          hideDocumentUploadModal();
        }
        return code;
      }
    },
    [uploadDocument, hideDocumentUploadModal],
  );

  return {
    documentUploadLoading: loading,
    onDocumentUploadOk,
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
  };
};
