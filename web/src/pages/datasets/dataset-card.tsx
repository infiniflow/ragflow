import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IDataset } from '@/interfaces/database/dataset';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { ChevronRight } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { DatasetDropdown } from './dataset-dropdown';
import useDuplicateDataset from '@/ee/pages/datasets/use-duplicate-dataset';
import { useRenameDataset } from './use-rename-dataset';

export type DatasetCardProps = {
  dataset: IDataset;
  showPrivilegeModal(): void;
} & Pick<ReturnType<typeof useRenameDataset>, 'showDatasetRenameModal'> & {
    showDatasetDuplicateModal: ReturnType<
      typeof useDuplicateDataset
    >['showModal'];
  };

export function DatasetCard({
  dataset,
  showDatasetRenameModal,
  showDatasetDuplicateModal,
  showPrivilegeModal,
}: DatasetCardProps) {
  const { t } = useTranslation();
  const { navigateToDataset } = useNavigatePage();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  const handleCardClick = useCallback(() => {
    setPrimarySidebarCollapsed(true);
    navigateToDataset(dataset.id)();
  }, [dataset.id, navigateToDataset, setPrimarySidebarCollapsed]);

  return (
    <HomeCard
      data={{
        ...dataset,
        description: `${dataset.document_count} ${t('knowledgeDetails.files')}`,
      }}
      moreDropdown={
        <DatasetDropdown
          showDatasetRenameModal={showDatasetRenameModal}
          showDatasetDuplicateModal={showDatasetDuplicateModal}
          showPrivilegeModal={showPrivilegeModal}
          dataset={dataset}
        >
          <MoreButton></MoreButton>
        </DatasetDropdown>
      }
      sharedBadge={<SharedBadge>{dataset.nickname}</SharedBadge>}
      onClick={handleCardClick}
    ></HomeCard>
  );
}

export function SeeAllCard() {
  const { navigateToDatasetList } = useNavigatePage();
  const { t } = useTranslation();

  return (
    <Card
      className="w-full flex-none h-full cursor-pointer"
      onClick={() => navigateToDatasetList({ isCreate: false })}
    >
      <CardContent className="p-2.5 pt-1 w-full h-full flex items-center justify-center gap-1.5 text-text-secondary">
        {t('common.seeAll')} <ChevronRight className="size-4" />
      </CardContent>
    </Card>
  );
}
