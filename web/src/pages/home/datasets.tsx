import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import { RenameDialog } from '@/components/rename-dialog';
import { CardSkeleton } from '@/components/ui/skeleton';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchNextKnowledgeListByPage } from '@/hooks/use-knowledge-request';
import { useTranslation } from 'react-i18next';
import { DatasetCard } from '../datasets/dataset-card';
import { useRenameDataset } from '../datasets/use-rename-dataset';
import { SeeAllAppCard } from './application-card';
import { HomeCardGrid, SectionHeading } from './home-layout';

export function Datasets() {
  const { t } = useTranslation();
  const { kbs, loading } = useFetchNextKnowledgeListByPage();
  const {
    datasetRenameLoading,
    initialDatasetName,
    onDatasetRenameOk,
    datasetRenameVisible,
    hideDatasetRenameModal,
    showDatasetRenameModal,
  } = useRenameDataset();
  const { navigateToDatasetList } = useNavigatePage();

  return (
    <section className="mt-10">
      <SectionHeading iconName="datasets" label={t('header.dataset')} />

      <div>
        {loading ? (
          <div className="flex-1">
            <CardSkeleton />
          </div>
        ) : (
          <>
            {kbs?.length > 0 && (
              <HomeCardGrid>
                {kbs?.slice(0, 6).map((dataset) => (
                  <DatasetCard
                    key={dataset.id}
                    dataset={dataset}
                    showDatasetRenameModal={showDatasetRenameModal}
                  ></DatasetCard>
                ))}
                {
                  <SeeAllAppCard
                    click={() => navigateToDatasetList({ isCreate: false })}
                  ></SeeAllAppCard>
                }
              </HomeCardGrid>
            )}
            {!(kbs && kbs?.length > 0) && (
              <div className="w-[210px]">
                <EmptyAppCard
                  type={EmptyCardType.Dataset}
                  onClick={() => navigateToDatasetList({ isCreate: true })}
                />
              </div>
            )}
          </>
        )}
      </div>

      {datasetRenameVisible && (
        <RenameDialog
          hideModal={hideDatasetRenameModal}
          onOk={onDatasetRenameOk}
          initialName={initialDatasetName}
          loading={datasetRenameLoading}
        />
      )}
    </section>
  );
}
