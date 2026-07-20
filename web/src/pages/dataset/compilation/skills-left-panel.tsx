import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import { TreeDataItem, TreeView } from '@/components/ui/tree-view';
import {
  useDeleteDatasetSkillPage,
  useDeleteDatasetSkillTree,
  useFetchDatasetSkillTree,
} from '@/hooks/use-dataset-skill-request';
import { useDebounce } from 'ahooks';
import { FileText, Folder, Trash2 } from 'lucide-react';
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

type SkillDeleteActionProps = {
  skillKwd: string;
  deleteLoading: boolean;
  onDelete: (skillKwd: string) => void;
};

function SkillDeleteAction({
  skillKwd,
  deleteLoading,
  onDelete,
}: SkillDeleteActionProps) {
  const { t } = useTranslation();

  const handleTriggerClick = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      // TreeView does not guard action clicks: without this the row would
      // also get selected and a branch row would toggle its accordion.
      e.stopPropagation();
    },
    [],
  );

  const handleConfirmDelete = useCallback(() => {
    onDelete(skillKwd);
  }, [skillKwd, onDelete]);

  return (
    <ConfirmDeleteDialog
      title={t('datasetSkill.deleteSkillTitle')}
      content={{ title: t('datasetSkill.deleteSkillDescription') }}
      onOk={handleConfirmDelete}
    >
      <Button
        variant="ghost"
        size="icon-sm"
        disabled={deleteLoading}
        onClick={handleTriggerClick}
        // TreeActions keeps actions always visible on the selected row;
        // hide again so the button only appears while hovering the row.
        // `hidden` (not opacity-0) so no invisible click target remains.
        className="hidden group-hover:inline-flex"
      >
        <Trash2 />
      </Button>
    </ConfirmDeleteDialog>
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

  const renderSkillActions = useCallback(
    (skillKwd: string) => (
      <SkillDeleteAction
        skillKwd={skillKwd}
        deleteLoading={deletePageLoading}
        onDelete={handleDeleteSkill}
      />
    ),
    [deletePageLoading, handleDeleteSkill],
  );

  const treeData = useMemo(
    () => buildSkillTreeData(tree?.skill_with_weight, renderSkillActions),
    [tree?.skill_with_weight, renderSkillActions],
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
          {t('datasetSkill.folders')} ({totalCount})
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

      <div className="flex-1 min-h-0 overflow-y-auto px-1 pb-3">
        {loading && filteredTreeData.length === 0 ? (
          <div className="py-8 flex justify-center">
            <Spin size="small" />
          </div>
        ) : filteredTreeData.length === 0 ? (
          <div className="py-8 text-center text-sm text-text-secondary">
            {debouncedSearchString
              ? t('common.noData')
              : t('datasetSkill.empty')}
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
