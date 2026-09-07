import type {
  BusinessDocumentAssignableUser,
  BusinessDocumentCatalog,
  BusinessDocumentCommand,
  BusinessDocumentCommandResult,
  BusinessDocumentEvaPullResult,
  BusinessDocumentList,
  BusinessDocumentProjection,
  BusinessDocumentRevision,
  BusinessDocumentRole,
  CreateBusinessDocumentRequest,
  DeleteBusinessDocumentResult,
  EvaDocumentChange,
  EvaDocumentChangeList,
  EvaDocumentSourceSearchResult,
} from '@/pages/business-documents/types';
import api from '@/utils/api';
import request from '@/utils/next-request';
import axios, { AxiosRequestConfig } from 'axios';

type ApiEnvelope<T> = {
  code: number;
  data: T;
  message?: string;
};

type BusinessDocumentErrorPayload = {
  code?: number;
  message?: string;
  data?: {
    error_code?: string;
    details?: unknown;
    message?: string;
  };
};

function requestConfig(config: AxiosRequestConfig = {}): AxiosRequestConfig {
  return {
    ...config,
    // This page renders domain and retry errors in context. Avoid a duplicate
    // global toast from the shared Axios interceptor.
    skipErrorNotification: true,
  } as AxiosRequestConfig;
}

function unwrap<T>(payload: T | ApiEnvelope<T>): T {
  if (
    payload &&
    typeof payload === 'object' &&
    'code' in payload &&
    'data' in payload
  ) {
    const envelope = payload as ApiEnvelope<T>;
    if (envelope.code !== 0) {
      throw new Error(
        envelope.message ||
          'Не удалось выполнить запрос бизнес-документа: сервер не сообщил причину.',
      );
    }
    return envelope.data;
  }
  return payload as T;
}

export class BusinessDocumentConflictError extends Error {
  readonly code: string;
  readonly details: unknown;

  constructor(message: string, code = 'CONFLICT', details?: unknown) {
    super(message);
    this.name = 'BusinessDocumentConflictError';
    this.code = code;
    this.details = details;
  }
}

function requestFailureMessage(status?: number): string {
  if (status === undefined) {
    return 'Не удалось связаться с сервером. Проверьте подключение и повторите попытку.';
  }
  if (status === 401) {
    return 'Сессия истекла. Войдите в систему снова и повторите действие.';
  }
  if (status === 403) {
    return 'Недостаточно прав для этого действия. Если доступ необходим, обратитесь к администратору.';
  }
  if (status === 404) {
    return 'Запрошенный объект не найден. Обновите страницу и проверьте, что он ещё доступен.';
  }
  if (status >= 500) {
    return 'Сервер не смог выполнить запрос. Повторите попытку позже; если ошибка сохранится, обратитесь к администратору.';
  }
  return `Не удалось выполнить запрос документа (HTTP ${status}). Проверьте введённые данные и повторите действие.`;
}

function rethrowBusinessDocumentError(error: unknown): never {
  if (axios.isAxiosError(error)) {
    const payload = error.response?.data as
      | BusinessDocumentErrorPayload
      | undefined;
    const message = payload?.message || payload?.data?.message;
    if (error.response?.status === 409) {
      throw new BusinessDocumentConflictError(
        message || 'Команда конфликтует с состоянием документа.',
        payload?.data?.error_code || 'CONFLICT',
        payload?.data?.details,
      );
    }
    throw new Error(message || requestFailureMessage(error.response?.status));
  }
  throw error;
}

export async function createBusinessDocument(
  input: CreateBusinessDocumentRequest,
) {
  try {
    const response = await request.post(
      api.businessDocuments,
      input,
      requestConfig(),
    );
    return unwrap<BusinessDocumentProjection>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function listBusinessDocumentCatalog() {
  try {
    const response = await request.get(
      api.businessDocumentCatalog,
      requestConfig(),
    );
    return unwrap<BusinessDocumentCatalog>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function fetchBusinessDocument(documentId: string) {
  try {
    const response = await request.get(
      api.businessDocument(documentId),
      requestConfig(),
    );
    return unwrap<BusinessDocumentProjection>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function deleteBusinessDocument(documentId: string) {
  try {
    const response = await request.delete(
      api.businessDocument(documentId),
      requestConfig(),
    );
    return unwrap<DeleteBusinessDocumentResult>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function listBusinessDocumentRevisions(documentId: string) {
  try {
    const response = await request.get(
      api.businessDocumentRevisions(documentId),
      requestConfig(),
    );
    return unwrap<BusinessDocumentRevision[]>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function pullBusinessDocumentFromEva(
  documentId: string,
  expectedStateVersion: number,
) {
  try {
    const response = await request.post(
      api.businessDocumentEvaPull(documentId),
      { expected_state_version: expectedStateVersion },
      requestConfig(),
    );
    return unwrap<BusinessDocumentEvaPullResult>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function rebindBusinessDocumentToEva(
  documentId: string,
  expectedStateVersion: number,
) {
  try {
    const response = await request.post(
      api.businessDocumentEvaRebind(documentId),
      { expected_state_version: expectedStateVersion },
      requestConfig(),
    );
    return unwrap<BusinessDocumentProjection>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function createEvaChangeFromBusinessDocument(
  documentId: string,
  expectedStateVersion: number,
) {
  try {
    const response = await request.post(
      api.businessDocumentEvaChanges(documentId),
      { expected_state_version: expectedStateVersion },
      requestConfig(),
    );
    return unwrap<EvaDocumentChange>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function downloadBusinessDocumentExport(
  documentId: string,
  artifactId: string,
) {
  try {
    const response = await request.get(
      api.businessDocumentExportDownload(documentId, artifactId),
      requestConfig({ responseType: 'blob' }),
    );
    return response.data as Blob;
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function listBusinessDocuments(
  page = 1,
  pageSize = 20,
  scope: 'mine' | 'all' = 'all',
) {
  try {
    const response = await request.get(
      api.businessDocuments,
      requestConfig({ params: { page, page_size: pageSize, scope } }),
    );
    return unwrap<BusinessDocumentList>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function listBusinessDocumentAccessUsers() {
  try {
    const response = await request.get(
      api.businessDocumentAccessUsers,
      requestConfig(),
    );
    return unwrap<{ items: BusinessDocumentAssignableUser[] }>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function assignBusinessDocumentOwner(
  documentId: string,
  ownerId: string,
  expectedStateVersion: number,
) {
  try {
    const response = await request.put(
      api.businessDocumentOwner(documentId),
      { owner_id: ownerId, expected_state_version: expectedStateVersion },
      requestConfig(),
    );
    return unwrap<BusinessDocumentProjection>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function updateBusinessDocumentUserRole(
  userId: string,
  role: Exclude<BusinessDocumentRole, 'ADMIN'>,
) {
  try {
    const response = await request.patch(
      api.businessDocumentAccessUser(userId),
      { role },
      requestConfig(),
    );
    return unwrap<BusinessDocumentAssignableUser>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function submitBusinessDocumentCommand(
  documentId: string,
  command: BusinessDocumentCommand,
) {
  try {
    const response = await request.post(
      api.businessDocumentCommands(documentId),
      command,
      requestConfig(),
    );
    return unwrap<BusinessDocumentCommandResult>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function searchEvaDocumentSources(query: string) {
  try {
    const response = await request.get(
      api.evaBusinessDocumentSources,
      requestConfig({
        params: {
          query,
        },
      }),
    );
    return unwrap<EvaDocumentSourceSearchResult>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function createEvaDocumentChange(input: {
  connector_id: string;
  document_id: string;
  change_summary: string;
}) {
  try {
    const response = await request.post(
      api.evaBusinessDocumentChanges,
      input,
      requestConfig(),
    );
    return unwrap<EvaDocumentChange>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function listEvaDocumentChanges(page = 1, pageSize = 20) {
  try {
    const response = await request.get(
      api.evaBusinessDocumentChanges,
      requestConfig({ params: { page, page_size: pageSize } }),
    );
    return unwrap<EvaDocumentChangeList>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function fetchEvaDocumentChange(changeId: string) {
  try {
    const response = await request.get(
      api.evaBusinessDocumentChange(changeId),
      requestConfig(),
    );
    return unwrap<EvaDocumentChange>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export async function generateEvaDocumentChangeDraft(
  changeId: string,
  input: { expected_state_version: number; refinement?: string },
) {
  try {
    const response = await request.post(
      api.evaBusinessDocumentChangeGenerate(changeId),
      input,
      requestConfig(),
    );
    return unwrap<EvaDocumentChange>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

async function submitEvaDocumentChangeAction(
  url: string,
  expectedStateVersion: number,
  forceOverwrite = false,
) {
  try {
    const input: {
      expected_state_version: number;
      force_overwrite?: boolean;
    } = { expected_state_version: expectedStateVersion };
    if (forceOverwrite) input.force_overwrite = true;
    const response = await request.post(url, input, requestConfig());
    return unwrap<EvaDocumentChange>(response.data);
  } catch (error) {
    return rethrowBusinessDocumentError(error);
  }
}

export const approveEvaDocumentChange = (
  changeId: string,
  expectedStateVersion: number,
) =>
  submitEvaDocumentChangeAction(
    api.evaBusinessDocumentChangeApprove(changeId),
    expectedStateVersion,
  );

export const prepareEvaDocumentChange = (
  changeId: string,
  expectedStateVersion: number,
  forceOverwrite = false,
) =>
  submitEvaDocumentChangeAction(
    api.evaBusinessDocumentChangePrepare(changeId),
    expectedStateVersion,
    forceOverwrite,
  );

export const publishEvaDocumentChange = (
  changeId: string,
  expectedStateVersion: number,
  forceOverwrite = false,
) =>
  submitEvaDocumentChangeAction(
    api.evaBusinessDocumentChangePublish(changeId),
    expectedStateVersion,
    forceOverwrite,
  );
