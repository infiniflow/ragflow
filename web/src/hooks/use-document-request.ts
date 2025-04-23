import kbService from '@/services/knowledge-service';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { get } from 'lodash';
import { useParams } from 'umi';

export const enum DocumentApiAction {
  UploadDocument = 'uploadDocument',
  FetchDocumentList = 'fetchDocumentList',
}

export const useUploadNextDocument = () => {
  const queryClient = useQueryClient();
  const { id } = useParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.UploadDocument],
    mutationFn: async (fileList: File[]) => {
      const formData = new FormData();
      formData.append('kb_id', id!);
      fileList.forEach((file: any) => {
        formData.append('file', file);
      });

      try {
        const ret = await kbService.document_upload(formData);
        const code = get(ret, 'data.code');

        if (code === 0 || code === 500) {
          queryClient.invalidateQueries({
            queryKey: [DocumentApiAction.FetchDocumentList],
          });
        }
        return ret?.data;
      } catch (error) {
        console.warn(error);
        return {
          code: 500,
          message: error + '',
        };
      }
    },
  });

  return { uploadDocument: mutateAsync, loading, data };
};
