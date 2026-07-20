import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import {
  useDeleteDatasetSkillPage,
  useDeleteDatasetSkillTree,
  useFetchDatasetSkillTree,
} from '@/hooks/use-dataset-skill-request';
import { DatasetSkillTreeNode } from '@/interfaces/database/dataset-skill';
import { cn } from '@/lib/utils';
import { useDebounce } from 'ahooks';
import { Trash2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

type FlatSkillNode = {
  skill_kwd: string;
  depth: number;
};

type SkillsLeftPanelProps = {
  selectedSkill: string | null;
  onSelectSkill: (skillKwd: string | null) => void;
};

function flattenSkillTree(nodes: DatasetSkillTreeNode[] = []): FlatSkillNode[] {
  const result: FlatSkillNode[] = [];

  function walk(items: DatasetSkillTreeNode[], depth: number) {
    for (const item of items) {
      result.push({ skill_kwd: item.skill_kwd, depth });
      if (item.children_kwd?.length) {
        walk(item.children_kwd, depth + 1);
      }
    }
  }

  walk(nodes, 0);
  return result;
}

type SkillListItemProps = {
  item: FlatSkillNode;
  isSelected: boolean;
  deleteLoading: boolean;
  onSelect: (skillKwd: string) => void;
  onDelete: (skillKwd: string) => void;
};

function SkillListItem({
  item,
  isSelected,
  deleteLoading,
  onSelect,
  onDelete,
}: SkillListItemProps) {
  const { t } = useTranslation();

  const handleClick = useCallback(() => {
    onSelect(item.skill_kwd);
  }, [item.skill_kwd, onSelect]);

  const handleDeleteTriggerClick = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      e.stopPropagation();
    },
    [],
  );

  const handleConfirmDelete = useCallback(() => {
    onDelete(item.skill_kwd);
  }, [item.skill_kwd, onDelete]);

  return (
    <li
      onClick={handleClick}
      className={cn(
        'group flex items-center gap-2 px-3 py-2 rounded-md text-sm cursor-pointer',
        'text-text-secondary hover:bg-bg-base hover:text-text-primary',
        isSelected && 'bg-bg-card text-text-primary',
      )}
      style={{ paddingLeft: `${item.depth * 16 + 12}px` }}
    >
      <span className="flex-1 truncate">{item.skill_kwd}</span>
      <ConfirmDeleteDialog
        title={t('datasetSkill.deleteSkillTitle')}
        content={{ title: t('datasetSkill.deleteSkillDescription') }}
        onOk={handleConfirmDelete}
      >
        <Button
          variant="ghost"
          size="icon-sm"
          disabled={deleteLoading}
          onClick={handleDeleteTriggerClick}
          className="opacity-0 group-hover:opacity-100 shrink-0"
        >
          <Trash2 />
        </Button>
      </ConfirmDeleteDialog>
    </li>
  );
}

export function SkillsLeftPanel({
  selectedSkill,
  onSelectSkill,
}: SkillsLeftPanelProps) {
  const { t } = useTranslation();
  const { data: tree, loading } = useFetchDatasetSkillTree();
  const { deleteSkillTree, loading: deleteTreeLoading } =
    useDeleteDatasetSkillTree();
  const { deleteSkillPage, loading: deletePageLoading } =
    useDeleteDatasetSkillPage();
  const [searchString, setSearchString] = useState('');
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const allNodes = useMemo(
    () => flattenSkillTree(tree?.skill_with_weight),
    [tree?.skill_with_weight],
  );

  const filteredNodes = useMemo(() => {
    const keyword = debouncedSearchString.trim().toLowerCase();
    if (!keyword) return allNodes;
    return allNodes.filter((node) =>
      node.skill_kwd.toLowerCase().includes(keyword),
    );
  }, [allNodes, debouncedSearchString]);

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setSearchString(e.target.value);
    },
    [],
  );

  const handleDeleteAll = useCallback(async () => {
    const data = await deleteSkillTree();
    if (data?.code === 0) {
      onSelectSkill(null);
    }
  }, [deleteSkillTree, onSelectSkill]);

  const handleDeleteSkill = useCallback(
    async (skillKwd: string) => {
      const data = await deleteSkillPage(skillKwd);
      if (data?.code === 0 && selectedSkill === skillKwd) {
        onSelectSkill(null);
      }
    },
    [deleteSkillPage, selectedSkill, onSelectSkill],
  );

  return (
    <aside className="size-full flex flex-col">
      <section className="flex items-center justify-between px-3 pt-3">
        <span className="text-sm font-medium text-text-primary">
          {t('datasetSkill.folders')} ({allNodes.length})
        </span>
        <ConfirmDeleteDialog
          title={t('datasetSkill.deleteAllTitle')}
          content={{ title: t('datasetSkill.deleteAllDescription') }}
          onOk={handleDeleteAll}
        >
          <Button
            variant="ghost"
            size="icon-sm"
            disabled={deleteTreeLoading}
            data-testid="skills-clear-trigger"
          >
            <Trash2 />
          </Button>
        </ConfirmDeleteDialog>
      </section>

      <div className="px-3 py-2">
        <SearchInput
          placeholder={t('common.search')}
          value={searchString}
          onChange={handleSearchChange}
        />
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto px-3 pb-3">
        {loading && filteredNodes.length === 0 ? (
          <div className="py-8 flex justify-center">
            <Spin size="small" />
          </div>
        ) : filteredNodes.length === 0 ? (
          <div className="py-8 text-center text-sm text-text-secondary">
            {debouncedSearchString
              ? t('common.noData')
              : t('datasetSkill.empty')}
          </div>
        ) : (
          <ul className="space-y-1">
            {filteredNodes.map((item) => (
              <SkillListItem
                key={item.skill_kwd}
                item={item}
                isSelected={selectedSkill === item.skill_kwd}
                deleteLoading={deletePageLoading}
                onSelect={onSelectSkill}
                onDelete={handleDeleteSkill}
              />
            ))}
          </ul>
        )}
      </div>
    </aside>
  );
}
