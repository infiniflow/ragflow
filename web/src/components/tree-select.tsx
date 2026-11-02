import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import {
  ChevronDown,
  ChevronRight,
  Loader2,
  Search,
  TriangleAlert,
  X,
} from 'lucide-react';
import { forwardRef, useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  collectBranchIds,
  filterTreeNodes,
  withBranchIds,
  type TreeSelectNode,
} from './tree-select-utils';

export type { TreeSelectNode } from './tree-select-utils';

interface TreeSelectValueDisplayProps {
  selectedNode?: TreeSelectNode;
  missingValue?: string;
  renderSelected?: (node: TreeSelectNode | undefined) => React.ReactNode;
  renderMissingValue?: (value: string) => React.ReactNode;
  placeholder?: string;
}

function TreeSelectValueDisplay({
  selectedNode,
  missingValue,
  renderSelected,
  renderMissingValue,
  placeholder,
}: TreeSelectValueDisplayProps) {
  const { t } = useTranslation();

  let content: React.ReactNode;
  if (missingValue) {
    content = renderMissingValue ? (
      renderMissingValue(missingValue)
    ) : (
      <span className="flex items-center gap-1.5">
        <TriangleAlert className="size-4 flex-shrink-0" />
        <span className="truncate">{missingValue}</span>
      </span>
    );
  } else if (renderSelected) {
    content = renderSelected(selectedNode);
  } else {
    content = selectedNode?.title || placeholder || t('common.pleaseSelect');
  }

  return (
    <span className={cn('truncate', !selectedNode && 'text-text-secondary')}>
      {content}
    </span>
  );
}

interface TreeSelectRowsProps {
  nodes: TreeSelectNode[];
  level: number;
  expandedIds: Set<string>;
  loadingIds?: Set<string>;
  value?: string;
  onSelectNode: (node: TreeSelectNode) => void;
}

function TreeSelectRow({
  node,
  level,
  expandedIds,
  loadingIds,
  value,
  onSelectNode,
}: TreeSelectRowsProps & { node: TreeSelectNode }) {
  const leaf = !node.children?.length;
  const expanded = expandedIds.has(node.id);
  const selected = value === node.id;

  const handleClick = useCallback(() => {
    onSelectNode(node);
  }, [node, onSelectNode]);

  return (
    <div>
      <div
        className={cn(
          'flex items-center rounded-sm cursor-pointer text-sm',
          'hover:bg-accent transition-colors',
          !leaf && 'text-text-primary font-medium',
          selected && 'bg-accent text-accent-foreground',
          node.disabled && 'opacity-50 pointer-events-none',
        )}
        style={{
          paddingLeft: `${level * 20 + 4}px`,
          paddingRight: '8px',
          paddingTop: '6px',
          paddingBottom: '6px',
        }}
        onClick={handleClick}
      >
        <span className="w-4 h-4 mr-0.5 flex-shrink-0 flex items-center justify-center">
          {!leaf && (
            <>
              {loadingIds?.has(node.id) ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin text-text-secondary" />
              ) : expanded ? (
                <ChevronDown className="h-3.5 w-3.5 text-text-secondary" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5 text-text-secondary" />
              )}
            </>
          )}
        </span>
        <span className="truncate">{node.label ?? node.title}</span>
      </div>
      {!leaf && expanded && node.children && (
        <div>
          <TreeSelectNodeList
            nodes={node.children}
            level={level + 1}
            expandedIds={expandedIds}
            loadingIds={loadingIds}
            value={value}
            onSelectNode={onSelectNode}
          />
        </div>
      )}
    </div>
  );
}

function TreeSelectNodeList(props: TreeSelectRowsProps) {
  return (
    <>
      {props.nodes.map((node) => (
        <TreeSelectRow key={node.id} {...props} node={node} />
      ))}
    </>
  );
}

interface TreeSelectDropdownProps {
  showSearch?: boolean;
  searchTerm: string;
  onSearchTermChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  filteredData: TreeSelectNode[];
  expandedIds: Set<string>;
  loadingIds?: Set<string>;
  value?: string;
  onSelectNode: (node: TreeSelectNode) => void;
}

function TreeSelectDropdown({
  showSearch,
  searchTerm,
  onSearchTermChange,
  filteredData,
  expandedIds,
  loadingIds,
  value,
  onSelectNode,
}: TreeSelectDropdownProps) {
  const { t } = useTranslation();

  // Keep the popover from scrolling the page behind it.
  const handleWheel = useCallback((e: React.WheelEvent) => {
    e.stopPropagation();
  }, []);

  return (
    <>
      {showSearch && (
        <div className="flex items-center border-b px-3 py-2">
          <Search className="h-4 w-4 mr-2 flex-shrink-0 text-text-disabled" />
          <input
            type="text"
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-text-disabled"
            placeholder={t('common.search')}
            value={searchTerm}
            onChange={onSearchTermChange}
          />
        </div>
      )}
      <div className="max-h-60 overflow-auto p-1" onWheel={handleWheel}>
        {filteredData.length > 0 ? (
          <TreeSelectNodeList
            nodes={filteredData}
            level={0}
            expandedIds={expandedIds}
            loadingIds={loadingIds}
            value={value}
            onSelectNode={onSelectNode}
          />
        ) : (
          <div className="py-6 text-center text-sm text-slate-400">
            {t('common.noData')}
          </div>
        )}
      </div>
    </>
  );
}

interface TreeSelectProps {
  data: TreeSelectNode[];
  value?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  allowClear?: boolean;
  showSearch?: boolean;
  className?: string;
  defaultExpandAll?: boolean;
  renderSelected?: (node: TreeSelectNode | undefined) => React.ReactNode;
  /**
   * Custom display for a value that matches no node in `data` (e.g. the
   * referenced entity was deleted). Defaults to a warning icon plus the raw
   * value.
   */
  renderMissingValue?: (value: string) => React.ReactNode;
  /**
   * While true, an unmatched value is treated as not-yet-loaded instead of
   * missing, so the missing-value display doesn't flash before `data` arrives.
   */
  loading?: boolean;
  /**
   * Controlled expansion. When `expandedIds` is provided the component defers
   * to the caller and reports user toggles through `onExpandedChange`, which
   * lets the caller lazy-load children for freshly expanded branches. In
   * controlled mode `defaultExpandAll` and the built-in search auto-expansion
   * are disabled — the caller owns expansion and can combine `onSearchChange`
   * with filterTreeNodes to implement its own.
   */
  expandedIds?: Set<string>;
  onExpandedChange?: (ids: Set<string>) => void;
  /**
   * Reports search-box edits (event context), so a controlled-expansion
   * caller can auto-expand matching branches itself.
   */
  onSearchChange?: (term: string) => void;
  /**
   * Branch nodes currently loading their children; their expand chevron is
   * replaced by a spinner.
   */
  loadingIds?: Set<string>;
  /**
   * Extra content rendered in the trigger's trailing icon cluster, before the
   * clear/chevron icons (e.g. a status badge).
   */
  trailingAddon?: React.ReactNode;
  testId?: string;
}

export const TreeSelect = forwardRef<HTMLButtonElement, TreeSelectProps>(
  function TreeSelect(
    {
      data,
      value,
      onChange,
      placeholder,
      disabled,
      allowClear,
      showSearch,
      className,
      defaultExpandAll,
      renderSelected,
      renderMissingValue,
      loading,
      expandedIds: controlledExpandedIds,
      onExpandedChange,
      onSearchChange,
      loadingIds,
      trailingAddon,
      testId,
    },
    ref,
  ) {
    const [open, setOpen] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const [internalExpandedIds, setInternalExpandedIds] = useState<Set<string>>(
      new Set(),
    );

    const isExpansionControlled = controlledExpandedIds !== undefined;
    const expandedIds = isExpansionControlled
      ? controlledExpandedIds
      : internalExpandedIds;

    // Uncontrolled-only: in controlled mode the caller owns the expansion set.
    // Render-phase adjustment (React's endorsed "adjust state when a prop
    // changes" pattern) rather than an effect, so the expansion is settled
    // before paint. The previous-value guards keep each adjustment firing
    // only on a real change.
    const [prevExpandData, setPrevExpandData] = useState(data);
    if (data !== prevExpandData) {
      setPrevExpandData(data);
      if (defaultExpandAll && !isExpansionControlled) {
        setInternalExpandedIds(collectBranchIds(data));
      }
    }

    const selectedNode = useMemo(() => {
      const find = (nodes: TreeSelectNode[]): TreeSelectNode | undefined => {
        for (const node of nodes) {
          if (node.id === value && value) return node;
          if (node.children) {
            const found = find(node.children);
            if (found) return found;
          }
        }
      };
      return find(data);
    }, [data, value]);

    // A value matching no node means the referenced option is gone (e.g.
    // deleted) — unless `data` may simply not have loaded yet.
    const missingValue = value && !selectedNode && !loading ? value : undefined;

    const handleToggle = useCallback(
      (id: string) => {
        const next = new Set(expandedIds);
        if (next.has(id)) {
          next.delete(id);
        } else {
          next.add(id);
        }
        if (isExpansionControlled) {
          onExpandedChange?.(next);
        } else {
          setInternalExpandedIds(next);
        }
      },
      [expandedIds, isExpansionControlled, onExpandedChange],
    );

    const handleSelect = useCallback(
      (node: TreeSelectNode) => {
        if (node.disabled) return;
        if (node.children?.length) {
          handleToggle(node.id);
        } else {
          onChange?.(node.id);
          setOpen(false);
          setSearchTerm('');
        }
      },
      [onChange, handleToggle],
    );

    const handleClear = useCallback(
      (e: React.MouseEvent) => {
        e.stopPropagation();
        onChange?.('');
      },
      [onChange],
    );

    const filteredData = useMemo(
      () => filterTreeNodes(data, searchTerm),
      [data, searchTerm],
    );

    // Auto-expand all matching parents when the search term changes so users
    // can see search results (render-phase adjustment, same pattern as the
    // defaultExpandAll block above; withBranchIds returns the previous set
    // unchanged when nothing new was added, so React bails out of no-op
    // updates). Uncontrolled-only: controlled callers get `onSearchChange`
    // and own auto-expansion themselves.
    const [prevFilteredData, setPrevFilteredData] = useState(filteredData);
    if (filteredData !== prevFilteredData) {
      setPrevFilteredData(filteredData);
      if (searchTerm && !isExpansionControlled) {
        setInternalExpandedIds((prev) => withBranchIds(prev, filteredData));
      }
    }

    const handleSearchTermChange = useCallback(
      (e: React.ChangeEvent<HTMLInputElement>) => {
        setSearchTerm(e.target.value);
        onSearchChange?.(e.target.value);
      },
      [onSearchChange],
    );

    return (
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild disabled={disabled}>
          <button
            ref={ref}
            type="button"
            data-testid={testId}
            className={cn(
              'flex items-center justify-between border border-border-button rounded-md px-3 py-1.5 w-full',
              'bg-bg-input text-sm',
              'hover:bg-border-button transition-colors',
              disabled && 'opacity-50 cursor-not-allowed',
              className,
            )}
          >
            <TreeSelectValueDisplay
              selectedNode={selectedNode}
              missingValue={missingValue}
              renderSelected={renderSelected}
              renderMissingValue={renderMissingValue}
              placeholder={placeholder}
            />
            <div className="flex items-center ml-2 flex-shrink-0 gap-1">
              {trailingAddon}
              {allowClear && value ? (
                <X
                  className="h-4 w-4 opacity-50 hover:opacity-100"
                  onClick={handleClear}
                />
              ) : (
                <ChevronDown className="h-4 w-4 opacity-50" />
              )}
            </div>
          </button>
        </PopoverTrigger>
        <PopoverContent
          className="p-0 w-auto min-w-[var(--radix-popover-trigger-width)]"
          align="start"
          sideOffset={4}
        >
          <TreeSelectDropdown
            showSearch={showSearch}
            searchTerm={searchTerm}
            onSearchTermChange={handleSearchTermChange}
            filteredData={filteredData}
            expandedIds={expandedIds}
            loadingIds={loadingIds}
            value={value}
            onSelectNode={handleSelect}
          />
        </PopoverContent>
      </Popover>
    );
  },
);
