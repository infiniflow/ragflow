import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import { TreeDataItem, TreeView } from '@/components/ui/tree-view';
import { useFetchDatasetSkillTree } from '@/hooks/use-dataset-skill-request';
import { useDebounce } from 'ahooks';
import { FileText, Folder } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  buildSkillTreeData,
  countSkillTreeNodes,
  filterSkillTreeData,
} from './utils/skill-tree';

// TreeView only computes expandedItemIds when initialSelectedItemId is
// truthy; combined with expandAll, any truthy id makes every branch mount
// open. A sentinel that matches no real skill_kwd forces expand-all without
// highlighting any row as selected.
const ExpandAllSentinelId = '__skill-tree-expand-all-sentinel__';

type SkillsLeftPanelProps = {
  selectedSkill: string | null;
  onSelectSkill: (skillKwd: string | null) => void;
};

export function SkillsLeftPanel({
  selectedSkill: _selectedSkill,
  onSelectSkill,
}: SkillsLeftPanelProps) {
  const { t } = useTranslation();
  const { data: tree, loading } = useFetchDatasetSkillTree();
  const [searchString, setSearchString] = useState('');
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const totalCount = useMemo(
    () => countSkillTreeNodes(tree?.skill_with_weight),
    [tree?.skill_with_weight],
  );

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setSearchString(e.target.value);
    },
    [],
  );

  const treeData = useMemo(
    () => buildSkillTreeData(tree?.skill_with_weight),
    [tree?.skill_with_weight],
  );

  const filteredTreeData = useMemo(
    () => filterSkillTreeData(treeData, debouncedSearchString),
    [treeData, debouncedSearchString],
  );

  const handleTreeSelect = useCallback(
    (item: TreeDataItem | undefined) => {
      onSelectSkill(item?.id ?? null);
    },
    [onSelectSkill],
  );

  return (
    <aside className="size-full flex flex-col">
      <section className="flex items-center justify-between px-3 pt-3">
        <span className="text-sm font-medium text-text-primary">
          {t('knowledgeCompilation.skillFolders')} ({totalCount})
        </span>
      </section>

      <div className="px-3 py-2">
        <SearchInput
          placeholder={t('common.search')}
          value={searchString}
          onChange={handleSearchChange}
        />
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto px-1 pb-3">
        {loading && filteredTreeData.length === 0 ? (
          <div className="py-8 flex justify-center">
            <Spin size="small" />
          </div>
        ) : filteredTreeData.length === 0 ? (
          <div className="py-8 text-center text-sm text-text-secondary">
            {debouncedSearchString
              ? t('common.noData')
              : t('knowledgeCompilation.skillEmpty')}
          </div>
        ) : (
          <TreeView
            data={filteredTreeData}
            initialSelectedItemId={ExpandAllSentinelId}
            onSelectChange={handleTreeSelect}
            expandAll
            defaultNodeIcon={Folder}
            defaultLeafIcon={FileText}
          />
        )}
      </div>
    </aside>
  );
}
