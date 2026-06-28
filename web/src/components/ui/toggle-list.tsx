'use client';

import { cn } from '@/lib/utils';
import { ChevronsDown, Loader2, Search, X } from 'lucide-react';
import * as React from 'react';

export interface ToggleListOption<V = unknown> {
  /** Arbitrary value, including objects. Used as the React key when primitive. */
  value: V;
  /** Display content. Strings participate in local search filtering. */
  label: React.ReactNode;
  /** Per-item click handler. Whether the list closes afterwards is up to the caller. */
  onClick?: () => void;
}

export interface ToggleListLoadMore {
  /** Whether more items are available to load. When false, the button is hidden. */
  hasMore: boolean;
  /** Triggered when the user clicks the "Load more" button at the bottom of the list. */
  onLoadMore: () => void;
  /** Show a loading spinner inside the button and disable interaction. */
  loading?: boolean;
  /** Custom label for the button. Defaults to "Load more". */
  text?: React.ReactNode;
}

export interface ToggleListProps<V = unknown> {
  /** Class applied to the outer container that wraps the button and the list. */
  className?: string;
  /** Text (or any node) shown inside the trigger button. */
  btnText?: React.ReactNode;
  /** List items rendered inside the scrollable area. */
  options: ToggleListOption<V>[];
  /**
   * When true (default), clicking anywhere outside the component will close the list.
   * Set to false if the list should only be toggled by the button itself.
   */
  closeOnOutsideClick?: boolean;
  /** Max height (in px) of the scrollable list area. Defaults to 500. */
  maxHeight?: number;
  /** Class applied to the list container (the box around the search input + items). */
  listClassName?: string;
  /** Class applied to each list item. */
  itemClassName?: string;
  /** Class applied to the trigger button (merged with the default styles). */
  buttonClassName?: string;
  /** Placeholder rendered when `options` is empty (and no query is active). */
  emptyText?: React.ReactNode;
  /** Placeholder rendered when a search query yields no matches. */
  noResultsText?: React.ReactNode;

  /** Show a search input above the list. */
  searchable?: boolean;
  /** Placeholder for the search input. */
  searchPlaceholder?: string;
  /**
   * If provided, switches to API search mode: every query change calls this
   * callback. The component does NOT filter `options` locally — the caller is
   * expected to update `options` (typically after a debounce + fetch).
   * If omitted, the component performs a case-insensitive substring filter
   * locally against the stringified label.
   */
  onSearch?: (query: string) => void;
  /** Show a spinner inside the search input. Useful for API search loading state. */
  searchLoading?: boolean;
  /** Class applied to the search input wrapper. */
  searchClassName?: string;

  /**
   * Load-more pagination config. The caller owns the data; the component just
   * renders a button at the bottom of the list when `hasMore` is true or a
   * load is in flight.
   */
  loadMore?: ToggleListLoadMore;
  /**
   * Optional callback fired whenever the list is opened (`true`) or
   * closed (`false`). The component still owns the open/close state; this
   * is just a notification hook so callers can trigger side effects
   * (e.g. lazy-fetching list data) on first open.
   */
  onOpenChange?: (open: boolean) => void;
  /**
   * Optional footer slot rendered at the bottom of the dropdown, OUTSIDE
   * the scrollable items area. Stays pinned to the bottom of the panel
   * regardless of how many options are scrolled. Use it for actions
   * that should always be reachable (e.g. an "Add custom" button).
   *
   * The footer receives the current `open` state as a render prop so
   * callers can render a different node when the panel is collapsed.
   */
  footer?: React.ReactNode | ((state: { open: boolean }) => React.ReactNode);
  /** Class applied to the footer wrapper (e.g. for borders / background). */
  footerClassName?: string;
}

/**
 * ToggleList — a button that toggles a vertically stacked, scrollable list area
 * rendered directly below it (no portal/overlay). Click the button to expand,
 * click again to collapse.
 *
 * Features:
 * - The list occupies real DOM space and stretches to the parent's width
 *   (the trigger button keeps its natural width).
 * - Optional search input. Local mode (no `onSearch`) filters options client-side;
 *   API mode (with `onSearch`) only emits the query and lets the caller refetch.
 * - Optional load-more pagination. The caller owns the data; the component
 *   renders the trigger button.
 * - Optional `footer` slot rendered outside the scrollable items area,
 *   pinned to the bottom of the dropdown panel. Use it for actions that
 *   should always be reachable regardless of how many options are scrolled.
 *
 * Behavior notes:
 * - Clicking a list item calls that item's `onClick`. The component does not
 *   auto-close after a click — let the caller's onClick decide via controlled
 *   state if needed.
 * - External-click closing is opt-out via `closeOnOutsideClick={false}`.
 */
export function ToggleList<V = unknown>({
  className,
  btnText,
  options,
  closeOnOutsideClick = false,
  maxHeight = 500,
  listClassName,
  itemClassName,
  buttonClassName,
  emptyText = 'No options',
  noResultsText = 'No matching results',
  searchable = false,
  searchPlaceholder = 'Search…',
  onSearch,
  searchLoading = false,
  searchClassName,
  loadMore,
  onOpenChange,
  footer,
  footerClassName,
}: ToggleListProps<V>) {
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState('');
  const containerRef = React.useRef<HTMLDivElement>(null);
  const listId = React.useId();

  // Close on outside click
  React.useEffect(() => {
    if (!open || !closeOnOutsideClick) return;
    const handlePointerDown = (event: MouseEvent) => {
      const node = containerRef.current;
      if (node && !node.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handlePointerDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
    };
  }, [open, closeOnOutsideClick]);

  const isApiSearch = Boolean(onSearch);

  // Local search: case-insensitive substring filter on the stringified label.
  // In API search mode the caller is responsible for filtering, so we pass through.
  const filteredOptions = React.useMemo(() => {
    if (isApiSearch || !query.trim()) return options;
    const q = query.trim().toLowerCase();
    return options.filter(
      (opt) =>
        String(opt.label ?? '')
          .toLowerCase()
          .includes(q) ||
        String(opt.value ?? '')
          .toLowerCase()
          .includes(q),
    );
  }, [options, query, isApiSearch]);

  const handleQueryChange = React.useCallback(
    (next: string) => {
      setQuery(next);
      if (onSearch) onSearch(next);
    },
    [onSearch],
  );

  const showNoResults =
    searchable && query.trim().length > 0 && filteredOptions.length === 0;
  const showEmpty = !showNoResults && options.length === 0;

  // Resolve the footer node. Render-prop form gives callers the current
  // `open` state so they can render a different node when collapsed.
  const footerNode = typeof footer === 'function' ? footer({ open }) : footer;

  return (
    <div ref={containerRef} className={cn('flex flex-col', className)}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={listId}
        onClick={() =>
          setOpen((prev) => {
            const next = !prev;
            onOpenChange?.(next);
            return next;
          })
        }
        className={cn(
          'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded text-sm',
          'h-8 px-3 bg-bg-card text-text-secondary border border-border-button self-start',
          'hover:bg-border-button hover:text-text-primary',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary/40',
          buttonClassName,
        )}
      >
        {btnText}
        {typeof btnText === 'string' && (
          <ChevronsDown
            className={cn(
              'size-4 shrink-0 transition-transform duration-200',
              open && 'rotate-180',
            )}
            aria-hidden="true"
          />
        )}
      </button>
      {open && (
        <div
          id={listId}
          role="list"
          className={cn(
            'mt-1 flex flex-col rounded-md border border-border-button bg-bg-card w-full overflow-hidden',
            listClassName,
          )}
        >
          {searchable && (
            <div
              className={cn(
                'flex items-center gap-2 px-2 h-9 bg-bg-card m-2 rounded-md py-1',
                searchClassName,
              )}
            >
              <Search className="size-4 shrink-0 text-text-secondary" />
              <input
                type="text"
                value={query}
                onChange={(e) => handleQueryChange(e.target.value)}
                placeholder={searchPlaceholder}
                className="flex-1 min-w-0 bg-transparent text-sm outline-none placeholder:text-text-secondary/60"
              />
              {searchLoading && (
                <Loader2 className="size-4 shrink-0 animate-spin text-text-secondary" />
              )}
              {query && !searchLoading && (
                <button
                  type="button"
                  aria-label="Clear search"
                  onClick={() => handleQueryChange('')}
                  className="text-text-secondary hover:text-text-primary"
                >
                  <X className="size-4" />
                </button>
              )}
            </div>
          )}
          <div
            className="flex-1 overflow-y-auto divide-y divide-border-button"
            style={{ maxHeight }}
          >
            {showEmpty && (
              <div className="px-3 py-2 text-sm text-text-secondary">
                {emptyText}
              </div>
            )}
            {showNoResults && (
              <div className="px-3 py-2 text-sm text-text-secondary">
                {noResultsText}
              </div>
            )}
            {!showEmpty &&
              !showNoResults &&
              filteredOptions.map((option, index) => {
                const raw = option.value;
                // Use index as the key for objects (no stable identity) and
                // for nullish values; stringify primitives for a stable key.
                const key =
                  raw === null ||
                  raw === undefined ||
                  (typeof raw === 'object' && raw !== null)
                    ? index
                    : String(raw);
                return (
                  <div
                    key={key}
                    role="listitem"
                    onClick={() => {
                      option.onClick?.();
                    }}
                    className={cn(
                      'cursor-pointer px-3 py-4 text-sm hover:bg-border-button ',
                      itemClassName,
                    )}
                  >
                    {option.label}
                  </div>
                );
              })}
            {loadMore &&
              (loadMore.hasMore || loadMore.loading) &&
              filteredOptions.length > 0 && (
                <button
                  type="button"
                  disabled={!loadMore.hasMore || loadMore.loading}
                  onClick={loadMore.onLoadMore}
                  className={cn(
                    'w-full px-3 py-2 text-sm border-t border-border-button',
                    'text-text-secondary hover:text-text-primary hover:bg-border-button',
                    'disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-transparent',
                  )}
                >
                  {loadMore.loading ? (
                    <span className="inline-flex items-center justify-center gap-2">
                      <Loader2 className="size-3.5 animate-spin" /> Loading…
                    </span>
                  ) : (
                    (loadMore.text ?? 'Load more')
                  )}
                </button>
              )}
          </div>
          {footerNode && (
            <div
              className={cn(
                'shrink-0 border-t border-border-button ',
                footerClassName,
              )}
            >
              {footerNode}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
