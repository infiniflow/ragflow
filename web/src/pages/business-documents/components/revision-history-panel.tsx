import { Button } from '@/components/ui/button';
import {
  CircleHelp,
  FileClock,
  Lightbulb,
  Link2,
  MessageSquareText,
  Sparkles,
  X,
} from 'lucide-react';
import type {
  BusinessDocumentRevision,
  BusinessDocumentRevisionBasis,
} from '../types';

const basisIcons: Record<
  BusinessDocumentRevisionBasis['type'],
  typeof Sparkles
> = {
  INITIAL_DRAFT: Sparkles,
  QUESTION: CircleHelp,
  PROPOSAL: Lightbulb,
  COMMENT: MessageSquareText,
  EVA_SYNC: Link2,
};

function formatRevisionTime(value?: number | null) {
  if (!value) return 'Время не сохранено';
  const milliseconds = value < 1_000_000_000_000 ? value * 1000 : value;
  const date = new Date(milliseconds);
  return Number.isNaN(date.valueOf())
    ? 'Время не сохранено'
    : date.toLocaleString('ru-RU', {
        day: '2-digit',
        month: 'short',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      });
}

function formatUserIdentity(name?: string | null, login?: string | null) {
  const normalizedName = name?.trim();
  const normalizedLogin = login?.trim();
  if (normalizedName && normalizedLogin) {
    return `${normalizedName} (${normalizedLogin})`;
  }
  if (normalizedLogin) {
    return `Пользователь (${normalizedLogin})`;
  }
  return normalizedName || 'Пользователь недоступен';
}

interface RevisionHistoryPanelProps {
  revisions: BusinessDocumentRevision[];
  selectedRevisionId?: string;
  currentRevisionId?: string;
  loading?: boolean;
  error?: Error | null;
  onSelect: (revision: BusinessDocumentRevision) => void;
  onClose: () => void;
}

export function RevisionHistoryPanel({
  revisions,
  selectedRevisionId,
  currentRevisionId,
  loading,
  error,
  onSelect,
  onClose,
}: RevisionHistoryPanelProps) {
  const ordered = [...revisions].sort(
    (left, right) => right.revision_number - left.revision_number,
  );

  return (
    <aside
      className="min-h-0 animate-in overflow-y-auto border-s border-border-button bg-bg-base fade-in slide-in-from-right-2 duration-200 scrollbar-auto"
      data-testid="business-document-history"
      aria-label="История изменений документа"
    >
      <header className="sticky top-0 z-10 flex min-h-16 items-start justify-between gap-3 border-b border-border-button bg-bg-base px-5 py-4">
        <div>
          <div className="flex items-center gap-2">
            <FileClock className="size-4 text-accent-primary" />
            <h2 className="text-sm font-semibold">История изменений</h2>
          </div>
          <p className="mt-1 text-xs leading-5 text-text-secondary">
            Вопросы, комментарии и предложения, вошедшие в каждую ревизию.
          </p>
        </div>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label="Закрыть историю"
          onClick={onClose}
        >
          <X className="size-4" />
        </Button>
      </header>

      {loading && (
        <p className="px-5 py-8 text-sm text-text-secondary">
          Загружаем ревизии…
        </p>
      )}
      {error && (
        <p className="border-s-2 border-state-error px-5 py-4 text-sm text-state-error">
          {error.message}
        </p>
      )}
      {!loading && !error && !ordered.length && (
        <p className="px-5 py-8 text-sm text-text-secondary">
          Ревизий пока нет.
        </p>
      )}

      <ol className="px-5 py-2">
        {ordered.map((revision) => {
          const selected = revision.revision_id === selectedRevisionId;
          const current = revision.revision_id === currentRevisionId;
          return (
            <li
              key={revision.revision_id}
              className="relative border-s border-border-button pb-6 ps-5 last:border-transparent"
            >
              <span
                className={`absolute -start-[5px] top-5 size-2.5 rounded-full border-2 border-bg-base ${
                  selected ? 'bg-accent-primary' : 'bg-text-disabled'
                }`}
              />
              <button
                type="button"
                className="w-full py-4 text-start outline-none transition-colors hover:text-accent-primary focus-visible:ring-1 focus-visible:ring-accent-primary"
                onClick={() => onSelect(revision)}
                data-testid={`business-document-revision-${revision.revision_number}`}
              >
                <span className="flex flex-wrap items-baseline justify-between gap-2">
                  <span className="text-sm font-semibold text-text-primary">
                    Ревизия {revision.revision_number}
                    {current ? ' · текущая' : ''}
                  </span>
                  <span className="text-[11px] text-text-disabled">
                    {formatRevisionTime(revision.created_at)}
                  </span>
                </span>
                {revision.author_id && (
                  <span className="mt-1 block text-[11px] text-text-disabled">
                    Автор изменений:{' '}
                    {formatUserIdentity(
                      revision.author_name,
                      revision.author_login,
                    )}
                  </span>
                )}
                {!revision.change_basis?.length && (
                  <span className="mt-2 block text-xs text-text-secondary">
                    Основания не сохранены.
                  </span>
                )}
                {!!revision.change_basis?.length && (
                  <span className="mt-3 block space-y-3">
                    {revision.change_basis.map((basis) => {
                      const Icon = basisIcons[basis.type];
                      return (
                        <span
                          key={basis.event_id}
                          className="grid grid-cols-[16px_minmax(0,1fr)] gap-2.5"
                          data-testid="business-document-revision-basis"
                        >
                          <Icon className="mt-0.5 size-4 text-accent-primary" />
                          <span className="min-w-0">
                            <span className="block text-xs font-medium text-text-primary">
                              {basis.title}
                              {basis.section_id
                                ? ` · раздел ${basis.section_id}`
                                : ''}
                            </span>
                            {basis.initiated_by_actor_id && (
                              <span className="mt-0.5 block text-[11px] text-text-disabled">
                                Инициировал:{' '}
                                {formatUserIdentity(
                                  basis.initiated_by_actor_name,
                                  basis.initiated_by_actor_login,
                                )}
                              </span>
                            )}
                            {basis.actor_id && (
                              <span className="mt-0.5 block text-[11px] text-text-disabled">
                                {basis.actor_type === 'USER'
                                  ? 'Изменил'
                                  : 'Исполнитель'}
                                :{' '}
                                {basis.actor_type === 'USER'
                                  ? formatUserIdentity(
                                      basis.actor_name,
                                      basis.actor_login,
                                    )
                                  : basis.actor_id}
                              </span>
                            )}
                            <span className="mt-0.5 block whitespace-pre-wrap text-xs leading-5 text-text-secondary">
                              {basis.summary}
                            </span>
                            {basis.details && (
                              <span className="mt-1 block border-s border-border-button ps-2 text-[11px] leading-4 text-text-disabled">
                                {basis.details}
                              </span>
                            )}
                          </span>
                        </span>
                      );
                    })}
                  </span>
                )}
              </button>
            </li>
          );
        })}
      </ol>
    </aside>
  );
}
