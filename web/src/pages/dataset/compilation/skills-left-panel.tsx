import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import { useFetchDatasetSkillTree } from '@/hooks/use-dataset-skill-request';
import { DatasetSkillTreeNode } from '@/interfaces/database/dataset-skill';
import { cn } from '@/lib/utils';
import { useDebounce } from 'ahooks';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

type FlatSkillNode = {
  skill_kwd: string;
  depth: number;
};

type SkillsLeftPanelProps = {
  selectedSkill: string | null;
  onSelectSkill: (skillKwd: string) => void;
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
  onSelect: (skillKwd: string) => void;
};

function SkillListItem({ item, isSelected, onSelect }: SkillListItemProps) {
  const handleClick = useCallback(() => {
    onSelect(item.skill_kwd);
  }, [item.skill_kwd, onSelect]);

  return (
    <li
      onClick={handleClick}
      className={cn(
        'flex items-center gap-2 px-3 py-2 rounded-md text-sm cursor-pointer',
        'text-text-secondary hover:bg-bg-base hover:text-text-primary',
        isSelected && 'bg-bg-card text-text-primary',
      )}
      style={{ paddingLeft: `${item.depth * 16 + 12}px` }}
    >
      <span className="truncate">{item.skill_kwd}</span>
    </li>
  );
}

export function SkillsLeftPanel({
  selectedSkill,
  onSelectSkill,
}: SkillsLeftPanelProps) {
  const { t } = useTranslation();
  const { data: tree, loading } = useFetchDatasetSkillTree();
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

  return (
    <aside className="size-full flex flex-col">
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
                onSelect={onSelectSkill}
              />
            ))}
          </ul>
        )}
      </div>
    </aside>
  );
}
