import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { t } from 'i18next';
import { ChevronRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { DatasetDropdown } from './dataset-dropdown';
import { useRenameDataset } from './use-rename-dataset';

export type DatasetCardProps = {
  dataset: IKnowledge;
} & Pick<ReturnType<typeof useRenameDataset>, 'showDatasetRenameModal'>;

export function DatasetCard({
  dataset,
  showDatasetRenameModal,
}: DatasetCardProps) {
  const { navigateToDataset } = useNavigatePage();
  const { t } = useTranslation();

  return (
    <HomeCard
      data={{
        ...dataset,
        description: `${dataset.doc_num} ${t('knowledgeDetails.files')}`,
      }}
      moreDropdown={
        <DatasetDropdown
          showDatasetRenameModal={showDatasetRenameModal}
          dataset={dataset}
        >
          <MoreButton></MoreButton>
        </DatasetDropdown>
      }
      sharedBadge={<SharedBadge>{dataset.nickname}</SharedBadge>}
      onClick={navigateToDataset(dataset.id)}
    />
  );
}

export function SeeAllCard() {
  const { navigateToDatasetList } = useNavigatePage();

  return (
    <Card className="w-40 flex-none h-full" onClick={navigateToDatasetList}>
      <CardContent className="p-2.5 pt-1 w-full h-full flex items-center justify-center gap-1.5 text-text-secondary">
        {t('common.seeAll')} <ChevronRight className="size-4" />
      </CardContent>
    </Card>
  );
}
