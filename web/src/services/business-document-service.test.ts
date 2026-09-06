import {
  assignBusinessDocumentOwner,
  createEvaDocumentChange,
  generateEvaDocumentChangeDraft,
  listBusinessDocumentAccessUsers,
  listBusinessDocuments,
  prepareEvaDocumentChange,
  publishEvaDocumentChange,
  searchEvaDocumentSources,
  submitBusinessDocumentCommand,
  updateBusinessDocumentUserRole,
} from '@/services/business-document-service';
import api from '@/utils/api';
import request from '@/utils/next-request';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    put: jest.fn(),
  },
}));

const mockedGet = jest.mocked(request.get);
const mockedPost = jest.mocked(request.post);
const mockedPatch = jest.mocked(request.patch);
const mockedPut = jest.mocked(request.put);

beforeEach(() => jest.clearAllMocks());

test('loads the canonical paginated document list envelope', async () => {
  const list = {
    items: [
      {
        document_id: 'doc-1',
        title: 'Требования',
        lifecycle_state: 'INTAKE',
        operation_state: 'IDLE',
        state_version: 1,
        current_revision_number: null,
        update_time: 1_787_695_200,
      },
    ],
    total: 1,
    page: 2,
    page_size: 10,
  };
  mockedGet.mockResolvedValueOnce({ data: { code: 0, data: list } });

  await expect(listBusinessDocuments(2, 10, 'mine')).resolves.toEqual(list);
  expect(mockedGet).toHaveBeenCalledWith(api.businessDocuments, {
    params: { page: 2, page_size: 10, scope: 'mine' },
    skipErrorNotification: true,
  });
});

test('uses explicit access and ownership endpoints', async () => {
  const users = {
    items: [
      {
        user_id: 'author-2',
        nickname: 'Второй автор',
        role: 'AUTHOR_CREATOR' as const,
      },
    ],
  };
  const assigned = { document_id: 'doc-1', owner_id: 'author-2' };
  mockedGet.mockResolvedValueOnce({ data: { code: 0, data: users } });
  mockedPut.mockResolvedValueOnce({ data: { code: 0, data: assigned } });
  mockedPatch.mockResolvedValueOnce({
    data: { code: 0, data: users.items[0] },
  });

  await expect(listBusinessDocumentAccessUsers()).resolves.toEqual(users);
  await expect(
    assignBusinessDocumentOwner('doc-1', 'author-2', 7),
  ).resolves.toEqual(assigned);
  await expect(
    updateBusinessDocumentUserRole('author-2', 'AUTHOR_EDITOR'),
  ).resolves.toEqual(users.items[0]);

  expect(mockedGet).toHaveBeenCalledWith(api.businessDocumentAccessUsers, {
    skipErrorNotification: true,
  });
  expect(mockedPut).toHaveBeenCalledWith(
    api.businessDocumentOwner('doc-1'),
    { owner_id: 'author-2', expected_state_version: 7 },
    { skipErrorNotification: true },
  );
  expect(mockedPatch).toHaveBeenCalledWith(
    api.businessDocumentAccessUser('author-2'),
    { role: 'AUTHOR_EDITOR' },
    { skipErrorNotification: true },
  );
});

test('reads the domain error code from the backend error envelope', async () => {
  mockedPost.mockRejectedValueOnce({
    isAxiosError: true,
    message: 'Request failed with status code 409',
    response: {
      status: 409,
      data: {
        code: 409,
        message: 'Есть открытые вопросы',
        data: {
          error_code: 'OPEN_REVIEW_QUESTIONS',
          details: { question_ids: ['question-1'] },
        },
      },
    },
  });

  const promise = submitBusinessDocumentCommand('doc-1', {
    schema_version: '1',
    command_id: 'cmd-1',
    idempotency_key: 'idem-1',
    expected_state_version: 4,
    type: 'APPLY_CHANGES',
    payload: { base_revision_id: 'revision-1' },
  });

  await expect(promise).rejects.toMatchObject({
    name: 'BusinessDocumentConflictError',
    code: 'OPEN_REVIEW_QUESTIONS',
    message: 'Есть открытые вопросы',
    details: { question_ids: ['question-1'] },
  });
  expect(mockedPost).toHaveBeenCalledWith(
    api.businessDocumentCommands('doc-1'),
    expect.any(Object),
    { skipErrorNotification: true },
  );
});

test('uses a nested backend message without leaking an undefined transport error', async () => {
  mockedGet.mockRejectedValueOnce({
    isAxiosError: true,
    message: 'Request error 404: undefined',
    response: {
      status: 404,
      data: {
        code: 404,
        data: {
          error_code: 'DOCUMENT_NOT_FOUND',
          message: 'Бизнес-документ не найден',
        },
      },
    },
  });

  await expect(listBusinessDocuments()).rejects.toThrow(
    'Бизнес-документ не найден',
  );
});

test('localizes transport failures instead of exposing an English Axios message', async () => {
  mockedGet.mockRejectedValueOnce({
    isAxiosError: true,
    message: 'Network Error',
  });

  await expect(listBusinessDocuments()).rejects.toThrow(
    'Не удалось связаться с сервером. Проверьте подключение и повторите попытку.',
  );
});

test('explains a server failure when the backend returned no message', async () => {
  mockedGet.mockRejectedValueOnce({
    isAxiosError: true,
    message: 'Request failed with status code 503',
    response: { status: 503, data: {} },
  });

  await expect(listBusinessDocuments()).rejects.toThrow(
    'Сервер не смог выполнить запрос. Повторите попытку позже; если ошибка сохранится, обратитесь к администратору.',
  );
});

test('uses explicit EVA source, agent-generation and publish endpoints', async () => {
  const sourceResult = { items: [], connectors: [] };
  const change = { change_id: 'change-1' };
  mockedGet.mockResolvedValueOnce({ data: { code: 0, data: sourceResult } });
  mockedPost
    .mockResolvedValueOnce({ data: { code: 0, data: change } })
    .mockResolvedValueOnce({ data: { code: 0, data: change } })
    .mockResolvedValueOnce({ data: { code: 0, data: change } })
    .mockResolvedValueOnce({ data: { code: 0, data: change } });

  await expect(searchEvaDocumentSources('BR-42')).resolves.toEqual(
    sourceResult,
  );
  await createEvaDocumentChange({
    connector_id: 'connector-1',
    document_id: 'CmfDocument:doc-1',
    change_summary: 'Уточнить цель',
  });
  await generateEvaDocumentChangeDraft('change-1', {
    expected_state_version: 2,
    refinement: 'Сделать формулировку измеримой',
  });
  await prepareEvaDocumentChange('change-1', 4, true);
  await publishEvaDocumentChange('change-1', 5);

  expect(mockedGet).toHaveBeenCalledWith(api.evaBusinessDocumentSources, {
    params: { query: 'BR-42' },
    skipErrorNotification: true,
  });
  expect(mockedPost).toHaveBeenNthCalledWith(
    1,
    api.evaBusinessDocumentChanges,
    {
      connector_id: 'connector-1',
      document_id: 'CmfDocument:doc-1',
      change_summary: 'Уточнить цель',
    },
    { skipErrorNotification: true },
  );
  expect(mockedPost).toHaveBeenNthCalledWith(
    2,
    api.evaBusinessDocumentChangeGenerate('change-1'),
    {
      expected_state_version: 2,
      refinement: 'Сделать формулировку измеримой',
    },
    { skipErrorNotification: true },
  );
  expect(mockedPost).toHaveBeenNthCalledWith(
    3,
    api.evaBusinessDocumentChangePrepare('change-1'),
    { expected_state_version: 4, force_overwrite: true },
    { skipErrorNotification: true },
  );
  expect(mockedPost).toHaveBeenNthCalledWith(
    4,
    api.evaBusinessDocumentChangePublish('change-1'),
    { expected_state_version: 5 },
    { skipErrorNotification: true },
  );
});
