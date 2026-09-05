import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Routes } from '@/routes';
import {
  assignBusinessDocumentOwner,
  BusinessDocumentConflictError,
  createBusinessDocument,
  createEvaChangeFromBusinessDocument,
  deleteBusinessDocument,
  downloadBusinessDocumentExport,
  fetchBusinessDocument,
  listBusinessDocumentAccessUsers,
  listBusinessDocumentRevisions,
  listBusinessDocuments,
  pullBusinessDocumentFromEva,
  rebindBusinessDocumentToEva,
  submitBusinessDocumentCommand,
} from '@/services/business-document-service';
import {
  EvaUserCredentialStatus,
  listEvaUserCredentials,
} from '@/services/user-service';
import api from '@/utils/api';
import { downloadFileFromBlob } from '@/utils/file-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  Archive,
  ArrowDownToLine,
  ArrowRight,
  ArrowUpFromLine,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Download,
  ExternalLink,
  FileClock,
  FilePenLine,
  FilePlus2,
  Link2,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  Send,
  Sparkles,
  Trash2,
} from 'lucide-react';
import {
  FormEvent,
  MouseEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { DocumentPane } from './components/document-pane';
import {
  BusinessDocumentProgress,
  isBusinessDocumentOperationActive,
} from './components/document-progress';
import { EvaChangeCreatePanel } from './components/eva-change-create-panel';
import { EvaChangeWorkbench } from './components/eva-change-workbench';
import { ProtocolPane } from './components/protocol-pane';
import { RevisionHistoryPanel } from './components/revision-history-panel';
import { appendVoiceTranscript, VoiceInput } from './components/voice-input';
import type {
  BusinessDocumentCommand,
  BusinessDocumentCommandType,
  BusinessDocumentLifecycleState,
  BusinessDocumentOperationState,
  BusinessDocumentRevision,
  BusinessDocumentSelection,
} from './types';

const lifecycleLabels: Record<BusinessDocumentLifecycleState, string> = {
  INTAKE: 'Сбор вводных',
  REVIEW: 'Ревью',
  AGREED: 'Ревью пройдено',
  ARCHIVED: 'В архиве',
};

const operationLabels: Record<BusinessDocumentOperationState, string> = {
  IDLE: 'Готово к работе',
  ANALYZING: 'Анализ вводных',
  ANALYZING_REVIEW: 'Анализ замечаний',
  GENERATING_DRAFT: 'Создание черновика',
  APPLYING_CHANGES: 'Применение изменений',
  EXPORTING: 'Подготовка файла',
  FAILED: 'Операция завершилась с ошибкой',
};

const staleConflictCodes = new Set([
  'STATE_VERSION_CONFLICT',
  'BASE_REVISION_CONFLICT',
]);

function makeId(prefix: string) {
  const randomPart = Math.random().toString(36).slice(2, 10);
  return `${prefix}-${Date.now()}-${randomPart}`;
}

function formatUpdateTime(value: number | null) {
  if (!value) return 'Нет изменений';
  const milliseconds = value < 1_000_000_000_000 ? value * 1000 : value;
  const date = new Date(milliseconds);
  return Number.isNaN(date.valueOf())
    ? 'Дата неизвестна'
    : date.toLocaleString('ru-RU', {
        day: '2-digit',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit',
      });
}

const exportLabels = {
  MARKDOWN: 'Markdown',
  DOCX: 'Word',
  EVA_WIKI: 'EvaWiki',
} as const;

const BusinessDocumentKeys = {
  list: (page: number, scope: 'mine' | 'all') =>
    ['business-documents', scope, page] as const,
  detail: (documentId?: string) => ['business-document', documentId] as const,
  accessUsers: () => ['business-document-access-users'] as const,
};

function CreateBusinessDocumentPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<'new' | 'eva'>('new');
  const [title, setTitle] = useState('');
  const [idea, setIdea] = useState('');
  const [evaPageUrl, setEvaPageUrl] = useState('');
  const [page, setPage] = useState(1);
  const [scope, setScope] = useState<'mine' | 'all'>('mine');
  const documentsQuery = useQuery({
    queryKey: BusinessDocumentKeys.list(page, scope),
    queryFn: () => listBusinessDocuments(page, 20, scope),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.items.some((item) =>
        isBusinessDocumentOperationActive(item.operation_state),
      )
        ? 2000
        : false,
  });
  const canCreate = documentsQuery.data?.capabilities?.create !== false;
  const createMutation = useMutation({
    mutationFn: createBusinessDocument,
    onSuccess: async (document) => {
      const command: BusinessDocumentCommand = {
        schema_version: '1',
        command_id: makeId('cmd-initial-analysis'),
        idempotency_key: makeId('idem-initial-analysis'),
        expected_state_version: document.state_version,
        type: 'REQUEST_INTAKE_ASSESSMENT',
        payload: {},
      };

      try {
        await submitBusinessDocumentCommand(document.document_id, command);
      } finally {
        // The document already exists even if the follow-up request fails. Open
        // it so the author can retry the analysis without creating a duplicate.
        navigate(`${Routes.BusinessDocuments}/${document.document_id}`);
      }
    },
  });
  const deleteMutation = useMutation({
    mutationFn: (documentId: string) => deleteBusinessDocument(documentId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['business-documents'],
      });
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!title.trim() || !idea.trim() || createMutation.isPending) return;
    createMutation.mutate({
      schema_version: '1',
      document_type: 'business_requirements',
      title: title.trim(),
      idea: idea.trim(),
      dataset_ids: [],
      ...(evaPageUrl.trim() ? { eva_page_url: evaPageUrl.trim() } : {}),
    });
  };

  return (
    <main
      className="grid h-full min-h-0 grid-rows-[auto_1fr] bg-bg-base"
      data-testid="business-documents-create"
    >
      <header className="border-b border-border-button px-6 py-5 lg:px-8">
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary">
          Рабочие документы
        </h1>
        <p className="mt-1 text-sm text-text-secondary">
          Продолжите сохранённую работу или начните новые бизнес-требования.
        </p>
      </header>

      <div className="grid min-h-0 grid-cols-[minmax(0,1fr)_minmax(360px,440px)] max-lg:grid-cols-1 max-lg:overflow-y-auto">
        <section
          className="min-h-0 overflow-y-auto border-e border-border-button scrollbar-auto"
          data-testid="business-document-list"
          aria-label="Сохранённые документы"
        >
          <div className="flex min-h-11 flex-wrap items-center justify-between gap-3 border-b border-border-button px-6 py-2">
            <div className="flex items-center gap-3">
              <h2 className="text-sm font-medium">Сохранённые документы</h2>
              {documentsQuery.data && (
                <span className="text-xs text-text-secondary">
                  {documentsQuery.data.total}
                </span>
              )}
            </div>
            <div
              className="flex items-center gap-1"
              role="group"
              aria-label="Фильтр документов"
            >
              <Button
                size="sm"
                variant={scope === 'mine' ? 'secondary' : 'ghost'}
                onClick={() => {
                  setScope('mine');
                  setPage(1);
                }}
                data-testid="business-documents-filter-mine"
              >
                Мои
              </Button>
              <Button
                size="sm"
                variant={scope === 'all' ? 'secondary' : 'ghost'}
                onClick={() => {
                  setScope('all');
                  setPage(1);
                }}
                data-testid="business-documents-filter-all"
              >
                Все
              </Button>
            </div>
          </div>

          {documentsQuery.isLoading && (
            <div
              className="flex items-center gap-2 px-6 py-8 text-sm text-text-secondary"
              data-testid="business-document-list-loading"
            >
              <LoaderCircle className="size-4 animate-spin text-accent-primary" />
              Загружаем документы
            </div>
          )}
          {documentsQuery.error && (
            <div
              className="border-s-2 border-state-error px-5 py-4"
              data-testid="business-document-list-error"
            >
              <p className="text-sm text-state-error">
                {documentsQuery.error.message}
              </p>
              <Button
                size="sm"
                variant="ghost"
                className="mt-2"
                onClick={() => documentsQuery.refetch()}
              >
                <RefreshCw className="size-3.5" />
                Повторить
              </Button>
            </div>
          )}
          {deleteMutation.error && (
            <div
              className="border-s-2 border-state-error px-5 py-4 text-sm text-state-error"
              data-testid="business-document-delete-error"
            >
              {deleteMutation.error.message}
            </div>
          )}
          {!documentsQuery.isLoading &&
            !documentsQuery.error &&
            !documentsQuery.data?.items.length && (
              <div
                className="px-6 py-10 text-center"
                data-testid="business-document-list-empty"
              >
                <FilePlus2 className="mx-auto size-7 stroke-[1.25] text-text-disabled" />
                <p className="mt-3 text-sm font-medium">
                  {scope === 'mine'
                    ? 'У вас пока нет назначенных документов'
                    : 'Документов пока нет'}
                </p>
                <p className="mt-1 text-xs text-text-secondary">
                  {canCreate
                    ? 'Новый документ можно создать в форме справа.'
                    : 'Откройте фильтр «Все», чтобы просмотреть доступные документы.'}
                </p>
              </div>
            )}
          {documentsQuery.data?.items.map((item) => (
            <div
              key={item.document_id}
              className="group grid grid-cols-[minmax(0,1fr)_auto] items-center border-b border-border-button transition-colors hover:bg-bg-card"
            >
              <Link
                to={`${Routes.BusinessDocuments}/${item.document_id}`}
                className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-4 px-6 py-4 focus-visible:outline-none"
                data-testid="business-document-list-item"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="truncate text-sm font-medium text-text-primary">
                      {item.title}
                    </h3>
                    <Badge
                      variant={
                        item.lifecycle_state === 'AGREED'
                          ? 'success'
                          : 'secondary'
                      }
                    >
                      {lifecycleLabels[item.lifecycle_state]}
                    </Badge>
                    {item.owner_name && (
                      <span className="text-xs text-text-secondary">
                        Владелец: {item.owner_name}
                      </span>
                    )}
                  </div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-text-secondary">
                    <span>{formatUpdateTime(item.update_time)}</span>
                    <span>
                      {item.current_revision_number
                        ? `Ревизия ${item.current_revision_number}`
                        : 'Без черновика'}
                    </span>
                    {item.operation_state !== 'IDLE' && (
                      <BusinessDocumentProgress
                        compact
                        job={item.latest_job}
                        operationState={item.operation_state}
                        operationLabel={operationLabels[item.operation_state]}
                      />
                    )}
                  </div>
                </div>
                <ArrowRight className="size-4 text-text-disabled transition-transform group-hover:translate-x-0.5 group-hover:text-text-primary" />
              </Link>
              {item.permissions?.delete && (
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      className="me-4 text-text-secondary hover:text-state-error"
                      aria-label={`Удалить документ «${item.title}»`}
                      disabled={deleteMutation.isPending}
                      data-testid="business-document-delete"
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Удалить документ?</AlertDialogTitle>
                      <AlertDialogDescription>
                        Документ «{item.title}», его история, задания и файлы
                        экспорта будут удалены без возможности восстановления.
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>Отмена</AlertDialogCancel>
                      <AlertDialogAction
                        className="bg-state-error text-white hover:bg-state-error/90"
                        onClick={() => deleteMutation.mutate(item.document_id)}
                      >
                        Удалить
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              )}
            </div>
          ))}

          {documentsQuery.data && documentsQuery.data.total > 20 && (
            <div className="flex items-center justify-between gap-3 px-6 py-4">
              <span className="text-xs text-text-secondary">
                Страница {documentsQuery.data.page}
              </span>
              <div className="flex gap-2">
                <Button
                  size="icon-sm"
                  variant="outline"
                  aria-label="Предыдущая страница"
                  disabled={page <= 1 || documentsQuery.isFetching}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  <ChevronLeft className="size-4" />
                </Button>
                <Button
                  size="icon-sm"
                  variant="outline"
                  aria-label="Следующая страница"
                  disabled={
                    page * documentsQuery.data.page_size >=
                      documentsQuery.data.total || documentsQuery.isFetching
                  }
                  onClick={() => setPage((current) => current + 1)}
                >
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          )}
        </section>

        <section className="min-h-0 overflow-y-auto scrollbar-auto">
          <div className="sticky top-0 z-20 flex gap-1 border-b border-border-button bg-bg-base px-6 py-3 lg:px-8">
            <Button
              size="sm"
              variant={mode === 'new' ? 'secondary' : 'ghost'}
              onClick={() => setMode('new')}
              disabled={!canCreate}
              data-testid="new-document-mode"
            >
              <FilePlus2 className="size-4" />
              Новый
            </Button>
            <Button
              size="sm"
              variant={mode === 'eva' ? 'secondary' : 'ghost'}
              onClick={() => setMode('eva')}
              data-testid="eva-change-mode"
            >
              <FilePenLine className="size-4" />
              Доработать EVA
            </Button>
          </div>

          {mode === 'new' && !canCreate ? (
            <div
              className="px-6 py-10 lg:px-8"
              data-testid="business-document-create-denied"
            >
              <h2 className="text-lg font-semibold text-text-primary">
                Создание документов недоступно
              </h2>
              <p className="mt-2 text-sm leading-6 text-text-secondary">
                Ваша роль позволяет читать доступные документы и редактировать
                назначенные вам, но не создавать новые.
              </p>
            </div>
          ) : mode === 'new' ? (
            <form onSubmit={submit} className="px-6 py-7 lg:px-8">
              <div className="flex items-center gap-2 text-sm font-medium text-accent-primary">
                <FilePlus2 className="size-4" />
                Новый документ
              </div>
              <h2 className="mt-3 text-xl font-semibold tracking-tight text-text-primary">
                Бизнес-требования
              </h2>
              <p className="mt-2 text-sm leading-6 text-text-secondary">
                Агент соберёт вводные, подготовит документ и проведёт
                согласование.
              </p>

              <label className="mt-6 block space-y-2 text-sm font-medium">
                <span>Название</span>
                <Input
                  value={title}
                  maxLength={200}
                  aria-label="Название документа"
                  placeholder="Например, Переводы одной кнопкой"
                  onChange={(event) => setTitle(event.target.value)}
                  suffix={
                    <VoiceInput
                      label="Название"
                      onTranscript={(transcript) =>
                        setTitle((value) =>
                          appendVoiceTranscript(value, transcript, 200),
                        )
                      }
                      testId="voice-input-document-title"
                    />
                  }
                />
              </label>
              <label className="mt-5 block space-y-2 text-sm font-medium">
                <span>Идея</span>
                <div className="relative">
                  <Textarea
                    value={idea}
                    maxLength={10000}
                    autoSize={{ minRows: 7, maxRows: 18 }}
                    resize="vertical"
                    aria-label="Описание идеи"
                    placeholder="Что нужно создать, для кого и какой результат ожидается?"
                    className="pe-11"
                    onChange={(event) => setIdea(event.target.value)}
                  />
                  <div className="absolute end-2 top-2">
                    <VoiceInput
                      label="Идея"
                      onTranscript={(transcript) =>
                        setIdea((value) =>
                          appendVoiceTranscript(value, transcript, 10000),
                        )
                      }
                      testId="voice-input-document-idea"
                    />
                  </div>
                </div>
              </label>

              <label className="mt-5 block space-y-2 text-sm font-medium">
                <span>Страница EVA</span>
                <Input
                  type="url"
                  value={evaPageUrl}
                  maxLength={2048}
                  aria-label="URL страницы EVA"
                  placeholder="https://eva.example.com/project/Document/BR-42"
                  onChange={(event) => setEvaPageUrl(event.target.value)}
                />
                <span className="block text-xs font-normal leading-5 text-text-secondary">
                  Необязательно. При доступном коннекторе появится обмен
                  изменениями в обе стороны; иначе сохранится ссылка на
                  страницу.
                </span>
              </label>

              {createMutation.error && (
                <p className="mt-4 text-sm text-state-error" role="alert">
                  {createMutation.error.message}
                </p>
              )}

              <div className="mt-6 flex scroll-mt-20 justify-end">
                <Button
                  type="submit"
                  variant="accent"
                  size="lg"
                  loading={createMutation.isPending}
                  disabled={!title.trim() || !idea.trim()}
                >
                  <Sparkles className="size-4" />
                  Начать работу
                </Button>
              </div>
            </form>
          ) : (
            <EvaChangeCreatePanel />
          )}
        </section>
      </div>
    </main>
  );
}

export default function BusinessDocumentsPage() {
  const { id: documentId, changeId } = useParams<{
    id: string;
    changeId: string;
  }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [selection, setSelection] = useState<BusinessDocumentSelection | null>(
    null,
  );
  const [hasConflict, setHasConflict] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [selectedRevisionId, setSelectedRevisionId] = useState<string>();
  const [evaSyncNotice, setEvaSyncNotice] = useState<string>();
  const [ownerSelection, setOwnerSelection] = useState('');
  const clearSelection = useCallback(() => setSelection(null), []);

  const documentQuery = useQuery({
    queryKey: BusinessDocumentKeys.detail(documentId),
    queryFn: () => fetchBusinessDocument(documentId!),
    enabled: Boolean(documentId && !changeId),
    retry: false,
    refetchInterval: (query) => {
      const state = query.state.data?.operation_state;
      return state && state !== 'IDLE' && state !== 'FAILED' ? 2000 : false;
    },
  });

  const document = documentQuery.data;
  useEffect(() => {
    setOwnerSelection(document?.owner_id ?? '');
  }, [document?.owner_id]);
  const accessUsersQuery = useQuery({
    queryKey: BusinessDocumentKeys.accessUsers(),
    queryFn: listBusinessDocumentAccessUsers,
    enabled: Boolean(document?.permissions?.assign),
    retry: false,
  });
  const assignOwnerMutation = useMutation({
    mutationFn: () => {
      if (!documentId || !document || !ownerSelection) {
        throw new Error('Выберите пользователя');
      }
      return assignBusinessDocumentOwner(
        documentId,
        ownerSelection,
        document.state_version,
      );
    },
    onSuccess: async (updatedDocument) => {
      queryClient.setQueryData(
        BusinessDocumentKeys.detail(documentId),
        updatedDocument,
      );
      await queryClient.invalidateQueries({
        queryKey: ['business-documents'],
      });
    },
  });
  const deleteMutation = useMutation({
    mutationFn: () => {
      if (!documentId) throw new Error('Документ не загружен');
      return deleteBusinessDocument(documentId);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['business-documents'],
      });
      navigate(Routes.BusinessDocuments);
    },
  });
  const evaPageUrl = document?.eva_binding?.page_url;
  const evaConnectorId = document?.eva_binding?.connector_id;
  const evaCredentialsQuery = useQuery<EvaUserCredentialStatus[]>({
    queryKey: ['eva-user-credentials'],
    queryFn: async () => {
      const { data } = await listEvaUserCredentials();
      if (data.code !== 0) {
        throw new Error(
          data.message || 'Не удалось проверить персональный EVA-токен.',
        );
      }
      return data.data?.items ?? [];
    },
    enabled: Boolean(
      documentId &&
      !changeId &&
      evaPageUrl &&
      document?.permissions?.edit !== false,
    ),
    retry: false,
  });
  const hasPersonalEvaToken = useMemo(
    () =>
      Boolean(
        evaPageUrl &&
        evaCredentialsQuery.data?.some(
          (credential) =>
            credential.configured &&
            (!evaConnectorId ||
              credential.connector_id === evaConnectorId ||
              credential.connectors.some(
                (connector) => connector.id === evaConnectorId,
              )),
        ),
      ),
    [evaConnectorId, evaCredentialsQuery.data, evaPageUrl],
  );
  const currentRevisionId = document?.current_revision?.revision_id;
  useEffect(() => {
    clearSelection();
  }, [clearSelection, currentRevisionId]);
  const revisionsQuery = useQuery({
    queryKey: ['business-document-revisions', documentId],
    queryFn: () => listBusinessDocumentRevisions(documentId!),
    enabled: Boolean(documentId && !changeId && historyOpen),
    retry: false,
  });
  const commandMutation = useMutation({
    mutationFn: ({
      type,
      payload,
    }: {
      type: BusinessDocumentCommandType;
      payload: Record<string, unknown>;
    }) => {
      if (!documentId || !document) {
        throw new Error('Документ не загружен');
      }
      const command: BusinessDocumentCommand = {
        schema_version: '1',
        command_id: makeId('cmd'),
        idempotency_key: makeId('idem'),
        expected_state_version: document.state_version,
        type,
        payload,
      };
      return submitBusinessDocumentCommand(documentId, command);
    },
    onSuccess: async () => {
      setHasConflict(false);
      await queryClient.invalidateQueries({
        queryKey: BusinessDocumentKeys.detail(documentId),
      });
    },
    onError: (error) => {
      if (
        error instanceof BusinessDocumentConflictError &&
        staleConflictCodes.has(error.code)
      ) {
        setHasConflict(true);
        void documentQuery.refetch();
      } else {
        setHasConflict(false);
      }
    },
  });

  const submitCommand = useCallback(
    (
      type: BusinessDocumentCommandType,
      payload: Record<string, unknown>,
      onSuccess?: () => void,
    ) => commandMutation.mutate({ type, payload }, { onSuccess }),
    [commandMutation],
  );

  const allowed = useMemo(
    () =>
      new Set(
        document?.permissions?.edit === false
          ? []
          : (document?.allowed_commands ?? []),
      ),
    [document?.allowed_commands, document?.permissions?.edit],
  );
  const isBusy =
    commandMutation.isPending ||
    Boolean(
      document &&
      document.operation_state !== 'IDLE' &&
      document.operation_state !== 'FAILED',
    );
  const displayedRevision = useMemo<BusinessDocumentRevision | null>(() => {
    if (!document?.current_revision) return null;
    if (!historyOpen || !selectedRevisionId) return document.current_revision;
    return (
      revisionsQuery.data?.find(
        (revision) => revision.revision_id === selectedRevisionId,
      ) ?? document.current_revision
    );
  }, [
    document?.current_revision,
    historyOpen,
    revisionsQuery.data,
    selectedRevisionId,
  ]);
  const ensureEvaCapability = useCallback(
    async (capability: 'PULL_FROM_EVA' | 'CREATE_EVA_CHANGE') => {
      if (!documentId || !document) throw new Error('Документ не загружен');
      if (document.eva_binding?.capabilities.includes(capability)) {
        return document;
      }
      const updatedDocument = await rebindBusinessDocumentToEva(
        documentId,
        document.state_version,
      );
      queryClient.setQueryData(
        BusinessDocumentKeys.detail(documentId),
        updatedDocument,
      );
      if (!updatedDocument.eva_binding?.capabilities.includes(capability)) {
        throw new Error('Связанная страница EVA недоступна для этой операции.');
      }
      return updatedDocument;
    },
    [document, documentId, queryClient],
  );
  const pullEvaMutation = useMutation({
    mutationFn: async () => {
      if (!documentId) throw new Error('Документ не загружен');
      const connectedDocument = await ensureEvaCapability('PULL_FROM_EVA');
      return pullBusinessDocumentFromEva(
        documentId,
        connectedDocument.state_version,
      );
    },
    onSuccess: async (result) => {
      queryClient.setQueryData(
        BusinessDocumentKeys.detail(documentId),
        result.document,
      );
      setEvaSyncNotice(
        result.sync.changed
          ? 'Новая версия EVA добавлена в ревью. Запустите анализ замечаний.'
          : 'В EVA нет новых изменений после последней синхронизации.',
      );
      await queryClient.invalidateQueries({
        queryKey: ['business-document-revisions', documentId],
      });
    },
  });
  const rebindEvaMutation = useMutation({
    mutationFn: () => {
      if (!documentId || !document) throw new Error('Документ не загружен');
      return rebindBusinessDocumentToEva(documentId, document.state_version);
    },
    onSuccess: (updatedDocument) => {
      queryClient.setQueryData(
        BusinessDocumentKeys.detail(documentId),
        updatedDocument,
      );
      setEvaSyncNotice('Страница EVA подключена. Синхронизация доступна.');
    },
  });
  const createEvaChangeMutation = useMutation({
    mutationFn: async () => {
      if (!documentId) throw new Error('Документ не загружен');
      const connectedDocument = await ensureEvaCapability('CREATE_EVA_CHANGE');
      return createEvaChangeFromBusinessDocument(
        documentId,
        connectedDocument.state_version,
      );
    },
    onSuccess: (change) =>
      navigate(`${Routes.BusinessDocuments}/eva/${change.change_id}`),
  });

  if (changeId) return <EvaChangeWorkbench changeId={changeId} />;

  if (!documentId) return <CreateBusinessDocumentPage />;

  if (documentQuery.isLoading) {
    return (
      <main
        className="flex h-full items-center justify-center bg-bg-base"
        data-testid="business-document-loading"
      >
        <div className="flex items-center gap-3 text-sm text-text-secondary">
          <LoaderCircle className="size-5 animate-spin text-accent-primary" />
          Загружаем рабочий документ
        </div>
      </main>
    );
  }

  if (documentQuery.error || !document) {
    return (
      <main
        className="flex h-full items-center justify-center bg-bg-base p-6"
        data-testid="business-document-error"
      >
        <div className="max-w-md border-s-2 border-state-error ps-5">
          <AlertTriangle className="size-5 text-state-error" />
          <h1 className="mt-3 text-lg font-semibold">
            Не удалось открыть документ
          </h1>
          <p className="mt-2 text-sm text-text-secondary">
            {documentQuery.error?.message || 'Документ не найден'}
          </p>
          <Button
            className="mt-4"
            variant="outline"
            onClick={() => documentQuery.refetch()}
          >
            <RefreshCw className="size-4" />
            Повторить
          </Button>
        </div>
      </main>
    );
  }

  const runDocumentCommand = (
    type: BusinessDocumentCommandType,
    payload?: Record<string, unknown>,
  ) => {
    const revisionId = document.current_revision?.revision_id;
    const payloadByCommand: Partial<
      Record<BusinessDocumentCommandType, Record<string, unknown>>
    > = {
      APPLY_CHANGES: { base_revision_id: revisionId },
    };
    submitCommand(type, payload ?? payloadByCommand[type] ?? {});
  };

  const visibleExports =
    document.latest_exports?.filter((artifact) => artifact.format !== 'DOCX') ??
    [];

  const downloadExport = async (
    event: MouseEvent<HTMLAnchorElement>,
    artifactId: string,
    filename: string,
  ) => {
    event.preventDefault();
    const blob = await downloadBusinessDocumentExport(
      document.document_id,
      artifactId,
    );
    downloadFileFromBlob(blob, filename);
  };

  return (
    <main
      className="grid h-full min-h-0 min-w-0 grid-rows-[auto_auto_1fr] bg-bg-base"
      data-testid="business-document-workbench"
    >
      <header
        className="flex min-h-16 min-w-0 flex-wrap items-start justify-between gap-3 border-b border-border-button px-5 py-3 sm:items-center"
        data-testid="business-document-header"
      >
        <div className="w-full min-w-0 sm:w-auto sm:flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h1 className="min-w-0 break-words text-lg font-semibold tracking-tight sm:truncate">
              {document.title}
            </h1>
            <Badge
              variant={
                document.lifecycle_state === 'AGREED' ? 'success' : 'secondary'
              }
            >
              {lifecycleLabels[document.lifecycle_state]}
            </Badge>
            <span className="font-mono text-[11px] text-text-disabled">
              v{document.state_version}
            </span>
          </div>
          <p className="mt-1 text-xs text-text-secondary">
            {operationLabels[document.operation_state]}
          </p>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-text-secondary">
            <span>Владелец: {document.owner_name || 'не назначен'}</span>
            {document.permissions?.assign && (
              <>
                <div className="min-w-56">
                  <SelectWithSearch
                    value={ownerSelection}
                    onChange={setOwnerSelection}
                    options={(accessUsersQuery.data?.items ?? []).map(
                      (user) => ({
                        value: user.user_id,
                        label: `${user.nickname} (${user.email})`,
                      }),
                    )}
                    placeholder="Выберите владельца"
                    emptyData="Нет доступных пользователей"
                    disabled={
                      accessUsersQuery.isLoading ||
                      assignOwnerMutation.isPending
                    }
                    testId="business-document-owner-select"
                    optionTestIdPrefix="business-document-owner-option-"
                  />
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={
                    !ownerSelection ||
                    ownerSelection === document.owner_id ||
                    assignOwnerMutation.isPending
                  }
                  loading={assignOwnerMutation.isPending}
                  onClick={() => assignOwnerMutation.mutate()}
                  data-testid="business-document-assign-owner"
                >
                  Назначить
                </Button>
              </>
            )}
          </div>
          {(accessUsersQuery.error || assignOwnerMutation.error) && (
            <p className="mt-1 text-xs text-state-error" role="alert">
              {(accessUsersQuery.error || assignOwnerMutation.error)?.message}
            </p>
          )}
        </div>

        <div
          className="flex w-full min-w-0 flex-wrap items-center justify-start gap-2 sm:w-auto sm:justify-end"
          data-testid="business-document-actions"
        >
          {!!document.current_revision && (
            <Button
              size="sm"
              variant={historyOpen ? 'secondary' : 'ghost'}
              onClick={() => {
                setHistoryOpen((open) => !open);
                setSelectedRevisionId(undefined);
              }}
              data-testid="business-document-history-toggle"
            >
              <FileClock className="size-4" />
              История
            </Button>
          )}
          {allowed.has('REQUEST_INTAKE_ASSESSMENT') && (
            <Button
              size="sm"
              variant="outline"
              disabled={isBusy}
              onClick={() => runDocumentCommand('REQUEST_INTAKE_ASSESSMENT')}
            >
              <Sparkles className="size-4" />
              Проанализировать вводные
            </Button>
          )}
          {allowed.has('REQUEST_DRAFT') && (
            <Button
              size="sm"
              variant="accent"
              disabled={isBusy}
              onClick={() => runDocumentCommand('REQUEST_DRAFT')}
            >
              <FilePlus2 className="size-4" />
              Создать черновик
            </Button>
          )}
          {allowed.has('START_REVIEW') && (
            <Button
              size="sm"
              variant="outline"
              disabled={isBusy}
              onClick={() => runDocumentCommand('START_REVIEW')}
            >
              <RotateCcw className="size-4" />
              Начать новое ревью
            </Button>
          )}
          {allowed.has('REQUEST_REVIEW_ASSESSMENT') && (
            <Button
              size="sm"
              variant="outline"
              disabled={isBusy}
              onClick={() => runDocumentCommand('REQUEST_REVIEW_ASSESSMENT')}
            >
              <Sparkles className="size-4" />
              Проанализировать замечания
            </Button>
          )}
          {allowed.has('REQUEST_EXPORT') && (
            <>
              <Button
                size="sm"
                variant="outline"
                disabled={isBusy}
                onClick={() =>
                  runDocumentCommand('REQUEST_EXPORT', {
                    revision_id: document.current_revision?.revision_id,
                    format: 'MARKDOWN',
                  })
                }
              >
                <Download className="size-4" />
                Создать Markdown
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={isBusy}
                onClick={() =>
                  runDocumentCommand('REQUEST_EXPORT', {
                    revision_id: document.current_revision?.revision_id,
                    format: 'EVA_WIKI',
                  })
                }
              >
                <Send className="size-4" />
                Создать HTML для EvaWiki
              </Button>
            </>
          )}
          {allowed.has('APPLY_CHANGES') && (
            <Button
              size="sm"
              variant="accent"
              disabled={isBusy}
              loading={commandMutation.isPending}
              data-testid="apply-changes-button"
              onClick={() => runDocumentCommand('APPLY_CHANGES')}
            >
              <CheckCircle2 className="size-4" />
              Завершить ревью
            </Button>
          )}
          {allowed.has('ARCHIVE') && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button size="sm" variant="ghost" disabled={isBusy}>
                  <Archive className="size-4" />В архив
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Архивировать документ?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Работа с документом будет завершена. История и ревизии
                    останутся доступны для чтения.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Отмена</AlertDialogCancel>
                  <AlertDialogAction
                    className="bg-state-error text-white hover:bg-state-error/90"
                    onClick={() => runDocumentCommand('ARCHIVE')}
                  >
                    Архивировать
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
          {document.permissions?.delete && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-state-error"
                  disabled={isBusy || deleteMutation.isPending}
                  data-testid="business-document-delete-detail"
                >
                  <Trash2 className="size-4" />
                  Удалить
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Удалить документ?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Документ «{document.title}», его история, задания и файлы
                    экспорта будут удалены без возможности восстановления.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Отмена</AlertDialogCancel>
                  <AlertDialogAction
                    className="bg-state-error text-white hover:bg-state-error/90"
                    onClick={() => deleteMutation.mutate()}
                  >
                    Удалить
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
        </div>
      </header>

      <div aria-live="polite">
        {deleteMutation.error && (
          <div className="border-b border-state-error/40 bg-state-error/10 px-5 py-2 text-sm text-state-error">
            {deleteMutation.error.message}
          </div>
        )}
        {document.eva_binding && (
          <div
            className="flex animate-in flex-wrap items-center gap-x-4 gap-y-2 border-b border-border-button bg-bg-card/35 px-5 py-2.5 text-xs fade-in duration-200"
            data-testid="business-document-eva-binding"
          >
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <Link2 className="size-3.5 shrink-0 text-accent-primary" />
              <span className="text-text-secondary">Связано с EVA:</span>
              <a
                href={document.eva_binding.page_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex min-w-0 items-center gap-1 font-medium text-text-primary hover:text-accent-primary"
              >
                <span className="truncate">
                  {document.eva_binding.document_name ||
                    document.eva_binding.document_code ||
                    document.eva_binding.page_url}
                </span>
                <ExternalLink className="size-3 shrink-0" />
              </a>
              {document.eva_binding.status === 'LINK_ONLY' && (
                <>
                  <span className="text-text-disabled">
                    Только ссылка — доступный коннектор не найден
                  </span>
                  {document.permissions?.edit !== false && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={isBusy || rebindEvaMutation.isPending}
                      loading={rebindEvaMutation.isPending}
                      onClick={() => rebindEvaMutation.mutate()}
                      data-testid="rebind-business-document-to-eva"
                    >
                      <RefreshCw className="size-3.5" />
                      Подключить заново
                    </Button>
                  )}
                </>
              )}
            </div>
            {document.permissions?.edit !== false && hasPersonalEvaToken && (
              <Button
                size="sm"
                variant="ghost"
                disabled={
                  isBusy ||
                  !document.current_revision ||
                  !['REVIEW', 'AGREED'].includes(document.lifecycle_state) ||
                  pullEvaMutation.isPending
                }
                loading={pullEvaMutation.isPending}
                onClick={() => pullEvaMutation.mutate()}
                data-testid="pull-business-document-from-eva"
              >
                <ArrowDownToLine className="size-3.5" />
                Перечитать текущий документ из EVA
              </Button>
            )}
            {document.permissions?.edit !== false && hasPersonalEvaToken && (
              <Button
                size="sm"
                variant="ghost"
                disabled={
                  isBusy ||
                  document.lifecycle_state !== 'AGREED' ||
                  createEvaChangeMutation.isPending
                }
                loading={createEvaChangeMutation.isPending}
                onClick={() => createEvaChangeMutation.mutate()}
                data-testid="push-business-document-to-eva"
              >
                <ArrowUpFromLine className="size-3.5" />
                Сохранить текущий вариант в EVA
              </Button>
            )}
            {(rebindEvaMutation.error ||
              pullEvaMutation.error ||
              createEvaChangeMutation.error) && (
              <span className="w-full text-state-error" role="alert">
                {
                  (
                    rebindEvaMutation.error ||
                    pullEvaMutation.error ||
                    createEvaChangeMutation.error
                  )?.message
                }
              </span>
            )}
            {evaSyncNotice && (
              <span className="w-full text-text-secondary">
                {evaSyncNotice}
              </span>
            )}
          </div>
        )}
        {hasConflict && (
          <div
            className="flex flex-wrap items-center gap-3 border-b border-state-warning/40 bg-state-warning/5 px-5 py-2.5 text-sm"
            data-testid="business-document-conflict"
            role="alert"
          >
            <AlertTriangle className="size-4 shrink-0 text-state-warning" />
            <span className="min-w-0 flex-1">
              Документ изменился в другой вкладке. Состояние обновлено,
              проверьте решение ещё раз.
            </span>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setHasConflict(false);
                commandMutation.reset();
                void documentQuery.refetch();
              }}
            >
              Обновить
            </Button>
          </div>
        )}
        {commandMutation.error && !hasConflict && (
          <div
            className="flex items-center gap-3 border-b border-state-error/40 bg-state-error/5 px-5 py-2.5 text-sm text-state-error"
            data-testid="business-document-command-error"
            role="alert"
          >
            <AlertTriangle className="size-4 shrink-0" />
            {commandMutation.error.message}
          </div>
        )}
        {isBusinessDocumentOperationActive(document.operation_state) && (
          <BusinessDocumentProgress
            job={document.latest_job}
            operationState={document.operation_state}
            operationLabel={operationLabels[document.operation_state]}
          />
        )}
        {document.operation_state === 'FAILED' && document.last_error && (
          <div className="flex items-center gap-2 border-b border-state-error/30 bg-state-error/5 px-5 py-2 text-xs text-state-error">
            <Archive className="size-3.5" />
            {typeof document.last_error === 'string'
              ? document.last_error
              : document.last_error.message || document.last_error.code}
          </div>
        )}
        {!!visibleExports.length && (
          <div
            className="flex flex-wrap items-center gap-2 border-b border-border-button bg-bg-card/40 px-5 py-2 text-xs"
            data-testid="business-document-exports"
          >
            <span className="me-1 text-text-secondary">Готовые файлы:</span>
            {visibleExports.map((artifact) => (
              <a
                key={artifact.artifact_id}
                href={api.businessDocumentExportDownload(
                  document.document_id,
                  artifact.artifact_id,
                )}
                download={artifact.filename}
                onClick={(event) =>
                  void downloadExport(
                    event,
                    artifact.artifact_id,
                    artifact.filename,
                  )
                }
                className="inline-flex items-center gap-1.5 rounded-md border border-border-button bg-bg-base px-2.5 py-1.5 font-medium text-text-primary transition-colors hover:border-accent-primary hover:text-accent-primary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent-primary"
              >
                <Download className="size-3.5" />
                {exportLabels[artifact.format]}
                <span className="font-normal text-text-disabled">
                  r{artifact.revision_number ?? '—'}
                </span>
              </a>
            ))}
          </div>
        )}
      </div>

      <div className="grid min-h-0 grid-cols-[minmax(0,1fr)_minmax(360px,430px)] max-lg:grid-cols-1 max-lg:grid-rows-[minmax(360px,1fr)_minmax(320px,0.8fr)]">
        <DocumentPane
          revision={displayedRevision}
          onSelectionChange={(nextSelection) => {
            if (!historyOpen) setSelection(nextSelection);
          }}
        />
        {historyOpen ? (
          <RevisionHistoryPanel
            revisions={revisionsQuery.data ?? []}
            selectedRevisionId={
              selectedRevisionId ?? document.current_revision?.revision_id
            }
            currentRevisionId={document.current_revision?.revision_id}
            loading={revisionsQuery.isLoading}
            error={revisionsQuery.error}
            onSelect={(revision) => {
              setSelectedRevisionId(revision.revision_id);
              clearSelection();
            }}
            onClose={() => {
              setHistoryOpen(false);
              setSelectedRevisionId(undefined);
            }}
          />
        ) : (
          <ProtocolPane
            reviewCycle={document.protocol}
            reviewCycleNumber={document.active_review_cycle}
            proposalDecisionsOpen={document.lifecycle_state === 'REVIEW'}
            revision={document.current_revision}
            selection={selection}
            allowedCommands={[...allowed]}
            pending={isBusy}
            onCommand={submitCommand}
            onClearSelection={clearSelection}
          />
        )}
      </div>
    </main>
  );
}
