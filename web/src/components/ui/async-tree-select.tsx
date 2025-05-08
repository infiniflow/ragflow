import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react';
import { ReactNode, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './button';

type TreeId = number | string;

export type TreeNodeType = {
  id: TreeId;
  title: ReactNode;
  parentId: TreeId;
  isLeaf?: boolean;
};

type AsyncTreeSelectProps = {
  treeData: TreeNodeType[];
  value?: TreeId;
  onChange?(value: TreeId): void;
  loadData?(node: TreeNodeType): Promise<any>;
};

export function AsyncTreeSelect({
  treeData,
  value,
  loadData,
  onChange,
}: AsyncTreeSelectProps) {
  const [open, setOpen] = useState(false);
  const { t } = useTranslation();

  const [expandedKeys, setExpandedKeys] = useState<TreeId[]>([]);
  const [loadingId, setLoadingId] = useState<TreeId>('');

  const selectedTitle = useMemo(() => {
    return treeData.find((x) => x.id === value)?.title;
  }, [treeData, value]);

  const isExpanded = useCallback(
    (id: TreeId | undefined) => {
      if (id === undefined) {
        return true;
      }
      return expandedKeys.indexOf(id) !== -1;
    },
    [expandedKeys],
  );

  const handleNodeClick = useCallback(
    (id: TreeId) => (e: React.MouseEvent<HTMLLIElement>) => {
      e.stopPropagation();
      onChange?.(id);
      setOpen(false);
    },
    [onChange],
  );

  const handleArrowClick = useCallback(
    (node: TreeNodeType) => async (e: React.MouseEvent<HTMLButtonElement>) => {
      e.stopPropagation();
      const { id } = node;
      if (isExpanded(id)) {
        setExpandedKeys((keys) => {
          return keys.filter((x) => x !== id);
        });
      } else {
        const hasChild = treeData.some((x) => x.parentId === id);
        setExpandedKeys((keys) => {
          return [...keys, id];
        });

        if (!hasChild) {
          setLoadingId(id);
          await loadData?.(node);
          setLoadingId('');
        }
      }
    },
    [isExpanded, loadData, treeData],
  );

  const renderNodes = useCallback(
    (parentId?: TreeId) => {
      const currentLevelList = parentId
        ? treeData.filter((x) => x.parentId === parentId)
        : treeData.filter((x) => treeData.every((y) => x.parentId !== y.id));

      if (currentLevelList.length === 0) return null;

      return (
        <ul className={cn('pl-2', { hidden: !isExpanded(parentId) })}>
          {currentLevelList.map((x) => (
            <li
              key={x.id}
              onClick={handleNodeClick(x.id)}
              className="cursor-pointer  "
            >
              <div
                className={cn(
                  'flex justify-between items-center hover:bg-accent py-0.5 px-1 rounded-md ',
                  { 'bg-cyan-50': value === x.id },
                )}
              >
                <span className={cn('flex-1 ')}>{x.title}</span>
                {x.isLeaf || (
                  <Button
                    variant={'ghost'}
                    className="size-7"
                    onClick={handleArrowClick(x)}
                    disabled={loadingId === x.id}
                  >
                    {loadingId === x.id ? (
                      <Loader2 className="animate-spin" />
                    ) : isExpanded(x.id) ? (
                      <ChevronDown />
                    ) : (
                      <ChevronRight />
                    )}
                  </Button>
                )}
              </div>
              {renderNodes(x.id)}
            </li>
          ))}
        </ul>
      );
    },
    [handleArrowClick, handleNodeClick, isExpanded, loadingId, treeData, value],
  );

  useEffect(() => {
    if (isEmpty(treeData)) {
      loadData?.({ id: '', parentId: '', title: '' });
    }
  }, [loadData, treeData]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <div className="flex justify-between border px-2 py-1.5 rounded-md gap-2 items-center w-full">
          {selectedTitle || (
            <span className="text-slate-400">{t('common.pleaseSelect')}</span>
          )}
          <ChevronDown className="size-5 " />
        </div>
      </PopoverTrigger>
      <PopoverContent className="p-1 min-w-[var(--radix-popover-trigger-width)]">
        <ul>{renderNodes()}</ul>
      </PopoverContent>
    </Popover>
  );
}
