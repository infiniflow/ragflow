import request from '@/utils/request';
import fileManagerService, {
  downloadAgentFile,
  downloadDatasetDocument,
} from '../file-manager-service';

type ServiceMethod = { url: string; method: string; responseType?: string };

const mockRegisterServer = jest.fn((methods: Record<string, ServiceMethod>) => {
  const server: Record<string, jest.Mock> = {};
  for (const key of Object.keys(methods)) {
    server[key] = jest.fn((params, urlAppendix) => {
      const method = methods[key];
      let url = method.url;
      if (urlAppendix) {
        url += `/${urlAppendix}`;
      }
      return request.get(url, {
        ...method,
        params,
      });
    });
  }
  return server;
});

jest.mock('@/utils/api', () => ({
  __esModule: true,
  default: {
    listFile: '/api/v1/files',
    removeFile: '/api/v1/files',
    uploadFile: '/api/v1/files',
    getAllParentFolder: '/api/v1/files/parent-folders',
    createFolder: '/api/v1/files/folders',
    connectFileToKnowledge: '/api/v1/files/connect',
    getDocumentFile: '/api/v1/documents',
    getFile: '/api/v1/files',
    moveFile: '/api/v1/files/move',
    getDatasetDocumentFileDownload: (datasetId: string, docId: string) =>
      `/api/v1/datasets/${datasetId}/documents/${docId}`,
    getAttachmentFileDownload: (docId: string) =>
      `/api/v1/agents/attachments/${docId}/download`,
  },
}));

jest.mock('@/utils/register-server', () => ({
  __esModule: true,
  default: mockRegisterServer,
}));

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), { get: jest.fn() }),
}));

describe('file-manager-service', () => {
  const mockRequestGet = request.get as jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();
    mockRequestGet.mockResolvedValue({ data: 'blob' });
  });

  it('keeps every registered service URL present and binds document files by the camelCase API key', () => {
    const methods = mockRegisterServer.mock.calls[0][0] as Record<
      string,
      { url: string; method: string }
    >;

    expect(Object.values(methods).every(({ url }) => url.length > 0)).toBe(
      true,
    );
    expect(methods.getDocumentFile).toEqual({
      url: '/api/v1/documents',
      method: 'get',
      responseType: 'blob',
    });
    expect(fileManagerService.getDocumentFile).toBeDefined();
  });

  it('uses the current agent attachment download endpoint', async () => {
    await downloadAgentFile({ docId: 'doc-1', ext: 'pdf' });

    expect(mockRequestGet).toHaveBeenCalledWith(
      '/api/v1/agents/attachments/doc-1/download',
      { params: { ext: 'pdf' }, responseType: 'blob' },
    );
  });

  it('uses the current dataset document download endpoint', async () => {
    await downloadDatasetDocument({
      datasetId: 'dataset-1',
      docId: 'doc-1',
      ext: 'txt',
    });

    expect(mockRequestGet).toHaveBeenCalledWith(
      '/api/v1/datasets/dataset-1/documents/doc-1',
      { params: { ext: 'txt' }, responseType: 'blob' },
    );
  });
});
