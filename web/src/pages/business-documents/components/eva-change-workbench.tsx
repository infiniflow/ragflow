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
import { Textarea } from '@/components/ui/textarea';
import { Routes } from '@/routes';
import {
  approveEvaDocumentChange,
  BusinessDocumentConflictError,
  fetchEvaDocumentChange,
  generateEvaDocumentChangeDraft,
  prepareEvaDocumentChange,
  publishEvaDocumentChange,
} from '@/services/business-document-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  ExternalLink,
  FilePenLine,
  LoaderCircle,
  RefreshCw,
  Send,
  Sparkles,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import type {
  EvaDocumentChange,
  EvaDocumentChangeAction,
  EvaDocumentChangeState,
} from '../types';
import { appendVoiceTranscript, VoiceInput } from './voice-input';

const stateLabels: Record<EvaDocumentChangeState, string> = {
  EDITING: 'Черновик агента',
  GENERATING_DRAFT: 'Агент готовит черновик',
  APPROVED: 'Согласовано',
  PREPARING_EVA_DRAFT: 'Запись черновика в EVA',
  EVA_DRAFT_READY: 'Черновик сохранён в EVA',
  PUBLISHING: 'Публикация в EVA',
  PUBLISHED: 'Опубликовано',
};

const eventLabels: Record<string, string> = {
  CHANGE_REQUEST_CREATED: 'Зафиксирована исходная версия',
  AI_DRAFT_REQUESTED: 'Агент начал подготовку',
  AI_DRAFT_GENERATED: 'Агент подготовил доработку',
  DRAFT_APPROVED: 'Изменения согласованы',
  EVA_DRAFT_SAVED: 'Черновик записан в EVA',
  EVA_DOCUMENT_PUBLISHED: 'Документ опубликован в EVA',
  EXTERNAL_OPERATION_RETRIED: 'Внешняя операция возобновлена',
};

const busyStates = new Set<EvaDocumentChangeState>([
  'GENERATING_DRAFT',
  'PREPARING_EVA_DRAFT',
  'PUBLISHING',
]);

function updateCachedChange(
  queryClient: ReturnType<typeof useQueryClient>,
  changeId: string,
  change: EvaDocumentChange,
) {
  queryClient.setQueryData(['eva-document-change', changeId], change);
  void queryClient.invalidateQueries({ queryKey: ['eva-document-changes'] });
}

export function EvaChangeWorkbench({ changeId }: { changeId: string }) {
  const queryClient = useQueryClient();
  const [refinement, setRefinement] = useState('');
  const [view, setView] = useState<'draft' | 'diff'>('diff');
  const [hasConflict, setHasConflict] = useState(false);
  const [overwriteAction, setOverwriteAction] =
    useState<EvaDocumentChangeAction | null>(null);
  const changeQuery = useQuery({
    queryKey: ['eva-document-change', changeId],
    queryFn: () => fetchEvaDocumentChange(changeId),
    retry: false,
    refetchInterval: (query) =>
      query.state.data && busyStates.has(query.state.data.workflow_state)
        ? 2000
        : false,
  });
  const change = changeQuery.data;

  const actionMutation = useMutation({
    mutationFn: ({
      action,
      forceOverwrite = false,
    }: {
      action: EvaDocumentChangeAction;
      forceOverwrite?: boolean;
    }) => {
      if (!change) throw new Error('Доработка не загружена');
      if (action === 'GENERATE_DRAFT')
        return generateEvaDocumentChangeDraft(changeId, {
          expected_state_version: change.state_version,
          ...(refinement.trim() ? { refinement: refinement.trim() } : {}),
        });
      if (action === 'APPROVE')
        return approveEvaDocumentChange(changeId, change.state_version);
      if (action === 'PREPARE_EVA_DRAFT')
        return prepareEvaDocumentChange(
          changeId,
          change.state_version,
          forceOverwrite,
        );
      if (action === 'PUBLISH_EVA')
        return publishEvaDocumentChange(
          changeId,
          change.state_version,
          forceOverwrite,
        );
      throw new Error('Неизвестное действие');
    },
    onSuccess: (updated) => {
      setHasConflict(false);
      setOverwriteAction(null);
      setRefinement('');
      if (updated.diff.changed) setView('diff');
      updateCachedChange(queryClient, changeId, updated);
    },
    onError: async (error, variables) => {
      if (error instanceof BusinessDocumentConflictError) {
        if (
          error.code === 'EVA_SOURCE_VERSION_CONFLICT' &&
          ['PREPARE_EVA_DRAFT', 'PUBLISH_EVA'].includes(variables.action)
        ) {
          setHasConflict(false);
          await changeQuery.refetch();
          setOverwriteAction(variables.action);
          return;
        }
        setHasConflict(true);
        void changeQuery.refetch();
        return;
      }
      if (variables.action === 'GENERATE_DRAFT') void changeQuery.refetch();
    },
  });

  const allowed = useMemo(
    () => new Set(change?.allowed_actions ?? []),
    [change?.allowed_actions],
  );
  const isPending = actionMutation.isPending;
  const mutationError = actionMutation.error;

  if (changeQuery.isLoading) {
    return (
      <main className="flex h-full items-center justify-center bg-bg-base">
        <div className="flex items-center gap-3 text-sm text-text-secondary">
          <LoaderCircle className="size-5 animate-spin text-accent-primary" />
          Загружаем доработку EVA
        </div>
      </main>
    );
  }

  if (changeQuery.error || !change) {
    return (
      <main className="flex h-full items-center justify-center bg-bg-base p-6">
        <div className="max-w-md border-s-2 border-state-error ps-5">
          <AlertTriangle className="size-5 text-state-error" />
          <h1 className="mt-3 text-lg font-semibold">
            Не удалось открыть доработку
          </h1>
          <p className="mt-2 text-sm text-text-secondary">
            {changeQuery.error?.message || 'Доработка не найдена'}
          </p>
          <Button
            className="mt-4"
            variant="outline"
            onClick={() => changeQuery.refetch()}
          >
            <RefreshCw className="size-4" />
            Повторить
          </Button>
        </div>
      </main>
    );
  }

  return (
    <main
      className="grid h-full min-h-0 min-w-0 grid-rows-[auto_auto_1fr] bg-bg-base"
      data-testid="eva-change-workbench"
    >
      <header className="flex min-h-16 flex-wrap items-center justify-between gap-3 border-b border-border-button px-5 py-3">
        <div className="flex min-w-0 items-center gap-3">
          <Button
            size="icon-sm"
            variant="ghost"
            asChild
            aria-label="К списку документов"
          >
            <Link to={Routes.BusinessDocuments}>
              <ArrowLeft className="size-4" />
            </Link>
          </Button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-lg font-semibold tracking-tight">
                {change.source.document_name}
              </h1>
              <Badge
                variant={
                  change.workflow_state === 'PUBLISHED'
                    ? 'success'
                    : 'secondary'
                }
              >
                {stateLabels[change.workflow_state]}
              </Badge>
              <span className="font-mono text-[11px] text-text-disabled">
                v{change.state_version}
              </span>
            </div>
            <p className="mt-1 truncate text-xs text-text-secondary">
              Доработка существующего документа EVA
            </p>
          </div>
        </div>
        {change.source.web_url && (
          <Button size="sm" variant="outline" asChild>
            <a href={change.source.web_url} target="_blank" rel="noreferrer">
              <ExternalLink className="size-4" />
              Открыть EVA
            </a>
          </Button>
        )}
      </header>

      <div aria-live="polite">
        {hasConflict && (
          <div
            className="flex items-center gap-3 border-b border-state-warning/40 bg-state-warning/5 px-5 py-2.5 text-sm"
            role="alert"
          >
            <AlertTriangle className="size-4 shrink-0 text-state-warning" />
            <span className="min-w-0 flex-1">
              Состояние изменилось. Данные обновлены; проверьте diff и повторите
              действие.
            </span>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setHasConflict(false)}
            >
              Закрыть
            </Button>
          </div>
        )}
        {mutationError && !hasConflict && !overwriteAction && (
          <div
            className="flex items-center gap-3 border-b border-state-error/40 bg-state-error/5 px-5 py-2.5 text-sm text-state-error"
            role="alert"
          >
            <AlertTriangle className="size-4 shrink-0" />
            {mutationError.message}
          </div>
        )}
        {change.last_error && !mutationError && (
          <div
            className="flex items-center gap-3 border-b border-state-warning/40 bg-state-warning/5 px-5 py-2.5 text-sm"
            data-testid="eva-change-last-error"
          >
            <AlertTriangle className="size-4 shrink-0 text-state-warning" />
            {change.last_error.message || change.last_error.code}
          </div>
        )}
      </div>

      <div className="grid min-h-0 grid-cols-[minmax(260px,0.72fr)_minmax(420px,1.45fr)_minmax(300px,0.78fr)] max-xl:grid-cols-[minmax(240px,0.7fr)_minmax(420px,1.3fr)] max-xl:grid-rows-[minmax(0,1fr)_auto] max-lg:block max-lg:overflow-y-auto">
        <section
          className="min-h-0 overflow-y-auto border-e border-border-button scrollbar-auto"
          aria-label="Исходный документ"
        >
          <div className="sticky top-0 z-10 border-b border-border-button bg-bg-base px-5 py-3">
            <p className="text-xs font-medium uppercase tracking-wide text-text-secondary">
              Исходник EVA
            </p>
            <p className="mt-1 truncate text-sm font-medium">
              {change.source.document_code || change.source.document_id}
            </p>
            <p className="mt-1 break-all font-mono text-[10px] text-text-disabled">
              {change.source.base_version}
            </p>
          </div>
          <div className="border-b border-border-button px-5 py-4">
            <p className="text-xs font-medium text-text-secondary">
              Задача на доработку
            </p>
            <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-text-primary">
              {change.change_summary}
            </p>
          </div>
          <pre className="whitespace-pre-wrap break-words px-5 py-5 font-sans text-sm leading-6 text-text-secondary">
            {change.base_markdown}
          </pre>
        </section>

        <section
          className="flex min-h-0 flex-col border-e border-border-button"
          aria-label="Черновик и изменения"
        >
          <div className="flex h-12 shrink-0 items-center justify-between gap-3 border-b border-border-button px-4">
            <div className="flex items-center gap-1">
              <Button
                size="sm"
                variant={view === 'draft' ? 'secondary' : 'ghost'}
                onClick={() => setView('draft')}
              >
                Вариант агента
              </Button>
              <Button
                size="sm"
                variant={view === 'diff' ? 'secondary' : 'ghost'}
                onClick={() => setView('diff')}
              >
                Diff
                {change.diff.changed && (
                  <span className="ms-1 text-[11px] text-text-secondary">
                    {change.diff.changed_sections}
                  </span>
                )}
              </Button>
            </div>
          </div>

          {view === 'draft' ? (
            <div className="flex min-h-0 flex-1 flex-col p-4">
              <div className="min-h-0 flex-1 overflow-y-auto rounded-md border border-border-button bg-bg-card/40 p-4 scrollbar-auto">
                <pre
                  className="whitespace-pre-wrap break-words font-sans text-sm leading-6 text-text-primary"
                  data-testid="eva-agent-draft"
                >
                  {change.draft_markdown}
                </pre>
              </div>
              <p className="mt-3 text-xs leading-5 text-text-secondary">
                Текст формирует агент. Для корректировки результата уточните
                задачу и запустите подготовку повторно.
              </p>
            </div>
          ) : (
            <div
              className="min-h-0 flex-1 overflow-y-auto p-4 scrollbar-auto"
              data-testid="eva-change-diff"
            >
              {!change.diff.changed ? (
                <div className="border-s-2 border-border-button px-4 py-2">
                  <p className="text-sm font-medium">Изменений пока нет</p>
                  <p className="mt-1 text-xs text-text-secondary">
                    Отредактируйте и сохраните черновик.
                  </p>
                </div>
              ) : (
                <div className="space-y-5">
                  {change.diff.sections.map((section) => (
                    <section
                      key={section.key}
                      className="overflow-hidden rounded-md border border-border-button"
                    >
                      <h3 className="border-b border-border-button bg-bg-card px-3 py-2 text-xs font-medium">
                        {section.title}
                      </h3>
                      <div className="overflow-x-auto py-1 font-mono text-[12px] leading-5">
                        {section.lines.map((line, index) => (
                          <div
                            key={`${line.type}-${index}`}
                            className={`grid grid-cols-[24px_minmax(0,1fr)] px-2 ${
                              line.type === 'added'
                                ? 'bg-state-success/10 text-state-success'
                                : line.type === 'removed'
                                  ? 'bg-state-error/10 text-state-error'
                                  : 'text-text-secondary'
                            }`}
                          >
                            <span className="select-none text-center opacity-70">
                              {line.type === 'added'
                                ? '+'
                                : line.type === 'removed'
                                  ? '−'
                                  : ' '}
                            </span>
                            <span className="whitespace-pre-wrap break-words">
                              {line.content || ' '}
                            </span>
                          </div>
                        ))}
                      </div>
                    </section>
                  ))}
                </div>
              )}
            </div>
          )}
        </section>

        <aside
          className="min-h-0 overflow-y-auto px-5 py-5 scrollbar-auto max-xl:col-span-2 max-xl:border-t max-xl:border-border-button max-lg:border-t"
          aria-label="Подтвердить запись в EVA"
        >
          <div className="flex items-center gap-2">
            <FilePenLine className="size-4 text-accent-primary" />
            <h2 className="text-sm font-medium">Подтвердить запись в EVA</h2>
          </div>
          <div className="mt-4 grid grid-cols-3 gap-px overflow-hidden rounded-md border border-border-button bg-border-button text-center">
            <div className="bg-bg-base px-2 py-3">
              <span className="block text-base font-semibold text-state-success">
                +{change.diff.added_lines}
              </span>
              <span className="text-[10px] text-text-secondary">строк</span>
            </div>
            <div className="bg-bg-base px-2 py-3">
              <span className="block text-base font-semibold text-state-error">
                −{change.diff.removed_lines}
              </span>
              <span className="text-[10px] text-text-secondary">строк</span>
            </div>
            <div className="bg-bg-base px-2 py-3">
              <span className="block text-base font-semibold">
                {change.diff.changed_sections}
              </span>
              <span className="text-[10px] text-text-secondary">разделов</span>
            </div>
          </div>

          <ol
            className="mt-5 space-y-4 text-sm"
            aria-label="Этапы публикации в EVA"
          >
            <li className="flex gap-3">
              <span
                className={`mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full text-[10px] ${change.diff.changed ? 'bg-accent-primary text-white' : 'bg-bg-card text-text-disabled'}`}
              >
                1
              </span>
              <span>
                <span className="block font-medium">Сформировать черновик</span>
                <span className="mt-0.5 block text-xs text-text-secondary">
                  Агент формирует закрытый черновик по задаче автора.
                </span>
              </span>
            </li>
            <li className="flex gap-3">
              <span
                className={`mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full text-[10px] ${['APPROVED', 'EVA_DRAFT_READY', 'PUBLISHING', 'PUBLISHED'].includes(change.workflow_state) ? 'bg-accent-primary text-white' : 'bg-bg-card text-text-disabled'}`}
              >
                2
              </span>
              <span>
                <span className="block font-medium">
                  Сохранить как черновик в EVA
                </span>
                <span className="mt-0.5 block text-xs text-text-secondary">
                  Фиксируется точный hash черновика; опубликованная страница ещё
                  не меняется.
                </span>
              </span>
            </li>
            <li className="flex gap-3">
              <span
                className={`mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full text-[10px] ${change.workflow_state === 'PUBLISHED' ? 'bg-state-success text-white' : 'bg-bg-card text-text-disabled'}`}
              >
                3
              </span>
              <span>
                <span className="block font-medium">Опубликовать</span>
                <span className="mt-0.5 block text-xs text-text-secondary">
                  Перед публикацией EVA проверяется повторно.
                </span>
              </span>
            </li>
          </ol>

          <div className="mt-6 space-y-2">
            {allowed.has('GENERATE_DRAFT') && (
              <div className="mb-4 border-s-2 border-accent-primary ps-4">
                <label className="block text-xs font-medium text-text-secondary">
                  Уточнение для агента
                </label>
                <div className="relative mt-2">
                  <Textarea
                    value={refinement}
                    maxLength={10000}
                    autoSize={{ minRows: 3, maxRows: 8 }}
                    resize="vertical"
                    aria-label="Уточнение доработки EVA"
                    placeholder="Необязательно: уточните, что нужно изменить в следующем варианте."
                    className="pe-11"
                    disabled={isPending}
                    onChange={(event) => setRefinement(event.target.value)}
                  />
                  <div className="absolute end-2 top-2">
                    <VoiceInput
                      label="Уточнение доработки EVA"
                      disabled={isPending}
                      onTranscript={(transcript) =>
                        setRefinement((value) =>
                          appendVoiceTranscript(value, transcript, 10000),
                        )
                      }
                      testId="voice-input-eva-refinement"
                    />
                  </div>
                </div>
                <Button
                  className="mt-3 w-full"
                  variant="outline"
                  loading={
                    actionMutation.isPending &&
                    actionMutation.variables?.action === 'GENERATE_DRAFT'
                  }
                  disabled={isPending}
                  onClick={() =>
                    actionMutation.mutate({ action: 'GENERATE_DRAFT' })
                  }
                  data-testid="generate-eva-change-draft"
                >
                  <Sparkles className="size-4" />
                  {change.diff.changed
                    ? 'Пересобрать доработку'
                    : 'Сформировать доработку'}
                </Button>
              </div>
            )}
            {allowed.has('APPROVE') && (
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button
                    className="w-full"
                    variant="accent"
                    disabled={isPending}
                  >
                    <CheckCircle2 className="size-4" />
                    Подтвердить изменения
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Подтвердить текущие изменения?
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      Будет зафиксирована эта версия черновика. Любое
                      последующее редактирование потребует повторного
                      согласования.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Отмена</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={() =>
                        actionMutation.mutate({ action: 'APPROVE' })
                      }
                    >
                      Подтвердить изменения
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
            {allowed.has('PREPARE_EVA_DRAFT') && (
              <Button
                className="w-full"
                variant="accent"
                loading={actionMutation.isPending}
                disabled={isPending}
                onClick={() =>
                  actionMutation.mutate({ action: 'PREPARE_EVA_DRAFT' })
                }
                data-testid="prepare-eva-draft"
              >
                <Send className="size-4" />
                {change.workflow_state === 'PREPARING_EVA_DRAFT'
                  ? 'Повторить сохранение черновика'
                  : 'Сохранить как черновик в EVA'}
              </Button>
            )}
            {allowed.has('PUBLISH_EVA') && (
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button
                    className="w-full"
                    variant="accent"
                    disabled={isPending}
                    data-testid="publish-eva-document"
                  >
                    <Send className="size-4" />
                    {change.workflow_state === 'PUBLISHING'
                      ? 'Продолжить публикацию'
                      : 'Опубликовать в EVA'}
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Опубликовать изменения в EVA?
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      Это изменит видимый документ. Перед публикацией система
                      проверит исходную версию и сохранённый в EVA черновик.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Отмена</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={() =>
                        actionMutation.mutate({ action: 'PUBLISH_EVA' })
                      }
                    >
                      Опубликовать
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
          </div>

          <section className="mt-7 border-t border-border-button pt-5">
            <h3 className="text-xs font-medium uppercase tracking-wide text-text-secondary">
              Журнал
            </h3>
            <ol className="mt-3 space-y-3">
              {change.events.map((event) => (
                <li
                  key={event.event_id}
                  className="grid grid-cols-[8px_minmax(0,1fr)] gap-2 text-xs"
                >
                  <span className="mt-1 size-1.5 rounded-full bg-accent-primary" />
                  <span>
                    <span className="block text-text-primary">
                      {eventLabels[event.event_type] || event.event_type}
                    </span>
                    <span className="mt-0.5 block text-text-disabled">
                      #{event.sequence}
                    </span>
                  </span>
                </li>
              ))}
            </ol>
          </section>
        </aside>
      </div>
      <AlertDialog
        open={overwriteAction !== null}
        onOpenChange={(open) => {
          if (!open) setOverwriteAction(null);
        }}
      >
        <AlertDialogContent data-testid="eva-overwrite-confirmation">
          <AlertDialogHeader>
            <AlertDialogTitle>Перезаписать документ EVA?</AlertDialogTitle>
            <AlertDialogDescription>
              Опубликованный документ EVA изменился после создания этой заявки.
              Подтверждение заменит внешние изменения согласованной версией из
              системы «Агент Раггер».
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setOverwriteAction(null)}>
              Отмена
            </AlertDialogCancel>
            <AlertDialogAction
              data-testid="confirm-eva-overwrite"
              onClick={() => {
                const action = overwriteAction;
                setOverwriteAction(null);
                if (action)
                  actionMutation.mutate({ action, forceOverwrite: true });
              }}
            >
              Перезаписать EVA
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </main>
  );
}
