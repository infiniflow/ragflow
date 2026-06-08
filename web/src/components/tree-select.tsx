import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { ChevronDown, ChevronRight, Search, X } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

export interface TreeSelectNode {
  id: string;
  title: string;
  label?: React.ReactNode;
  children?: TreeSelectNode[];
  disabled?: boolean;
  data?: Record<string, any>;
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
  testId?: string;
}

export function TreeSelect({
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
  testId,
}: TreeSelectProps) {
  const [open, setOpen] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const { t } = useTranslation();

  useEffect(() => {
    if (!defaultExpandAll) return;
    const ids = new Set<string>();
    const walk = (nodes: TreeSelectNode[]) => {
      for (const node of nodes) {
        if (node.children?.length) {
          ids.add(node.id);
          walk(node.children);
        }
      }
    };
    walk(data);
    setExpandedIds(ids);
  }, [data, defaultExpandAll]);

  const selectedNode = useMemo(() => {
    const find = (nodes: TreeSelectNode[]): TreeSelectNode | undefined => {
      for (const node of nodes) {
        if (node.id === value) return node;
        if (node.children) {
          const found = find(node.children);
          if (found) return found;
        }
      }
    };
    return find(data);
  }, [data, value]);

  const isLeaf = useCallback(
    (node: TreeSelectNode) => !node.children?.length,
    [],
  );

  const handleToggle = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const handleSelect = useCallback(
    (node: TreeSelectNode) => {
      if (node.disabled) return;
      if (isLeaf(node)) {
        onChange?.(node.id);
        setOpen(false);
        setSearchTerm('');
      } else {
        handleToggle(node.id);
      }
    },
    [isLeaf, onChange, handleToggle],
  );

  const handleClear = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onChange?.('');
    },
    [onChange],
  );

  const filterTree = useCallback(
    (nodes: TreeSelectNode[], term: string): TreeSelectNode[] => {
      if (!term) return nodes;
      return nodes.reduce<TreeSelectNode[]>((acc, node) => {
        const titleMatch = node.title
          .toLowerCase()
          .includes(term.toLowerCase());
        const filteredChildren = node.children
          ? filterTree(node.children, term)
          : undefined;
        if (titleMatch || filteredChildren?.length) {
          acc.push({ ...node, children: filteredChildren ?? node.children });
        }
        return acc;
      }, []);
    },
    [],
  );

  const filteredData = useMemo(
    () => filterTree(data, searchTerm),
    [data, searchTerm, filterTree],
  );

  const visibleExpandedIds = useMemo(() => {
    if (!searchTerm) return expandedIds;
    const ids = new Set<string>();
    const walk = (nodes: TreeSelectNode[]) => {
      for (const node of nodes) {
        if (node.children?.length) {
          ids.add(node.id);
          walk(node.children);
        }
      }
    };
    walk(filteredData);
    return ids;
  }, [searchTerm, expandedIds, filteredData]);

  const renderTree = useCallback(
    (nodes: TreeSelectNode[], level = 0): React.ReactNode => {
      return nodes.map((node) => {
        const leaf = isLeaf(node);
        const expanded = visibleExpandedIds.has(node.id);
        const selected = value === node.id;

        return (
          <div key={node.id}>
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
              onClick={() => handleSelect(node)}
            >
              <span className="w-4 h-4 mr-0.5 flex-shrink-0 flex items-center justify-center">
                {!leaf && (
                  <>
                    {expanded ? (
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
              <div>{renderTree(node.children, level + 1)}</div>
            )}
          </div>
        );
      });
    },
    [isLeaf, visibleExpandedIds, value, handleSelect],
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild disabled={disabled}>
        <button
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
          <span className={cn('truncate', !selectedNode && 'text-slate-400')}>
            {renderSelected
              ? renderSelected(selectedNode)
              : selectedNode?.title || placeholder || t('common.pleaseSelect')}
          </span>
          <div className="flex items-center ml-2 flex-shrink-0">
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
        {showSearch && (
          <div className="flex items-center border-b px-3 py-2">
            <Search className="h-4 w-4 text-slate-400 mr-2 flex-shrink-0" />
            <input
              type="text"
              className="flex-1 bg-transparent text-sm outline-none placeholder:text-slate-400"
              placeholder={t('common.search')}
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
            />
          </div>
        )}
        <div
          className="max-h-60 overflow-auto p-1"
          onWheel={(e) => e.stopPropagation()}
        >
          {filteredData.length > 0 ? (
            renderTree(filteredData)
          ) : (
            <div className="py-6 text-center text-sm text-slate-400">
              {t('common.noData')}
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
