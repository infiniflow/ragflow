import { UploadFormSchemaType } from '@/components/file-upload-dialog';
import { useUploadBillingError } from '@/ee/hooks/use-upload-billing-error';
import { useSetModalState } from '@/hooks/common-hooks';
import { useUploadFile } from '@/hooks/use-file-request';
import { useCallback } from 'react';
import { useGetFolderId } from './hooks';

export const useHandleUploadFile = () => {
  const {
    visible: fileUploadVisible,
    hideModal: hideFileUploadModal,
    showModal: showFileUploadModal,
  } = useSetModalState();
  const { uploadFile, loading } = useUploadFile();
  const { handleUploadError } = useUploadBillingError();
  const id = useGetFolderId();

  const onFileUploadOk = useCallback(
    async ({ fileList }: UploadFormSchemaType): Promise<number | undefined> => {
      if (fileList.length > 0) {
        const ret = await uploadFile({
          fileList: fileList as File[],
          parentId: id,
        });

        if (handleUploadError(ret)) {
          return ret?.code;
        }

        if (ret?.code === 0) {
          hideFileUploadModal();
          return 0;
        }

        return ret?.code;
      }
    },
    [uploadFile, hideFileUploadModal, id, handleUploadError],
  );

  return {
    fileUploadLoading: loading,
    onFileUploadOk,
    fileUploadVisible,
    hideFileUploadModal,
    showFileUploadModal,
  };
};
