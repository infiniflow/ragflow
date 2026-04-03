import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  listFile,
  removeFile,
  uploadFile,
  getAllParentFolder,
  createFolder,
  connectFileToKnowledge,
  get_document_file,
  getFile,
  moveFile,
  get_document_file_download,
} = api;

const methods = {
  listFile: {
    url: listFile,
    method: 'get',
  },
  removeFile: {
    url: removeFile,
    method: 'delete',
  },
  uploadFile: {
    url: uploadFile,
    method: 'post',
  },
  getAllParentFolder: {
    url: getAllParentFolder,
    method: 'get',
  },
  createFolder: {
    url: createFolder,
    method: 'post',
  },
  connectFileToKnowledge: {
    url: connectFileToKnowledge,
    method: 'post',
  },
  getFile: {
    url: getFile,
    method: 'get',
    responseType: 'blob',
  },
  getDocumentFile: {
    url: get_document_file,
    method: 'get',
    responseType: 'blob',
  },
  moveFile: {
    url: moveFile,
    method: 'post',
  },
} as const;

const fileManagerService = registerServer<keyof typeof methods>(
  methods,
  request,
);

const shouldPreferLegacyFileApi = __API_PROXY_SCHEME__ !== 'go';
const isFileRestApi404 = (error: any) => error?.response?.status === 404;
const isFileRestApi404Response = (response: any) =>
  response?.status === 404 || response?.data?.code === 404;

const withLegacyFallback = async <T>(
  restCall: () => Promise<T>,
  legacyCall: () => Promise<T>,
): Promise<T> => {
  try {
    const response = await restCall();
    if (isFileRestApi404Response(response)) {
      return legacyCall();
    }
    return response;
  } catch (error) {
    if (isFileRestApi404(error)) {
      return legacyCall();
    }
    throw error;
  }
};

const withPreferredFileApi = async <T>(
  restCall: () => Promise<T>,
  legacyCall: () => Promise<T>,
): Promise<T> => {
  return shouldPreferLegacyFileApi
    ? withLegacyFallback(legacyCall, restCall)
    : withLegacyFallback(restCall, legacyCall);
};

export const listFileCompat = (params?: Record<string, any>) =>
  withPreferredFileApi(
    () => fileManagerService.listFile(params),
    () => request.get(api.legacyListFile, { params }),
  );

export const uploadFileCompat = (formData: FormData) =>
  withPreferredFileApi(
    () => fileManagerService.uploadFile(formData),
    () =>
      request(api.legacyUploadFile, {
        method: 'post',
        data: formData,
      }),
  );

export const createFolderCompat = (params: Record<string, any>) =>
  withPreferredFileApi(
    () => fileManagerService.createFolder(params),
    () =>
      request(api.legacyCreateFolder, {
        method: 'post',
        data: params,
      }),
  );

export const getAllParentFolderCompat = (fileId: string) =>
  withPreferredFileApi(
    () => fileManagerService.getAllParentFolder({}, `${fileId}/ancestors`),
    () =>
      request.get(api.legacyGetAllParentFolder, {
        params: { file_id: fileId },
      }),
  );

export const removeFileCompat = (ids: string[]) =>
  withPreferredFileApi(
    () => fileManagerService.removeFile({ ids }),
    () =>
      request(api.legacyRemoveFile, {
        method: 'post',
        data: { file_ids: ids },
      }),
  );

export const moveFileCompat = (params: {
  src_file_ids: string[];
  dest_file_id?: string;
  new_name?: string;
}) =>
  withPreferredFileApi(
    () => fileManagerService.moveFile(params),
    () => {
      if (
        params.new_name &&
        params.src_file_ids.length === 1 &&
        !params.dest_file_id
      ) {
        return request(api.legacyRenameFile, {
          method: 'post',
          data: {
            file_id: params.src_file_ids[0],
            name: params.new_name,
          },
        });
      }

      return request(api.legacyMoveFile, {
        method: 'post',
        data: params,
      });
    },
  );

export const getFileCompat = (id: string) =>
  withPreferredFileApi(
    () => fileManagerService.getFile({}, id),
    () =>
      request.get(api.legacyGetFile(id), {
        responseType: 'blob',
      }),
  );

export const downloadFile = (data: { docId: string; ext: string }) => {
  return request.get(get_document_file_download(data.docId), {
    params: { ext: data.ext },
    responseType: 'blob',
  });
};
export default fileManagerService;
