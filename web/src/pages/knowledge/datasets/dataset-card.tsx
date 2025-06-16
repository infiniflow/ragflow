import { MoreButton } from '@/components/more-button';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { formatDate } from '@/utils/date';
import { ChevronRight } from 'lucide-react';
import { DatasetDropdown } from './dataset-dropdown';
import { useDisplayOwnerName } from './use-display-owner';
import { useRenameDataset } from './use-rename-dataset';

export type DatasetCardProps = {
  dataset: IKnowledge;
} & Pick<ReturnType<typeof useRenameDataset>, 'showDatasetRenameModal'>;

export function DatasetCard({
  dataset,
  showDatasetRenameModal,
}: DatasetCardProps) {
  const { navigateToDataset } = useNavigatePage();
  const displayOwnerName = useDisplayOwnerName();

  const owner = displayOwnerName(dataset.tenant_id, dataset.nickname);

  return (
    <Card
      key={dataset.id}
      className="w-40"
      onClick={navigateToDataset(dataset.id)}
    >
      <CardContent className="p-2.5 pt-2 group">
        <section className="flex justify-between mb-2">
          <div className="flex gap-2 items-center">
            <Avatar className="size-6 rounded-lg">
              <AvatarImage src={dataset.avatar} />
              <AvatarFallback className="rounded-lg ">CN</AvatarFallback>
            </Avatar>
            {owner && (
              <Badge className="h-5 rounded-sm px-1 bg-background-badge text-text-badge">
                {owner}
              </Badge>
            )}
          </div>
          <DatasetDropdown
            showDatasetRenameModal={showDatasetRenameModal}
            dataset={dataset}
          >
            <MoreButton></MoreButton>
          </DatasetDropdown>
        </section>
        <div className="flex justify-between items-end">
          <div className="w-full">
            <h3 className="text-lg font-semibold mb-2 line-clamp-1">
              {dataset.name}
            </h3>
            <p className="text-xs text-text-sub-title">
              {dataset.doc_num} files
            </p>
            <p className="text-xs text-text-sub-title">
              {formatDate(dataset.update_time)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function SeeAllCard() {
  const { navigateToDatasetList } = useNavigatePage();

  return (
    <Card
      className="bg-colors-background-inverse-weak w-40"
      onClick={navigateToDatasetList}
    >
      <CardContent className="p-2.5 pt-1 w-full h-full flex items-center justify-center gap-1.5 text-text-sub-title">
        See All <ChevronRight className="size-4" />
      </CardContent>
    </Card>
  );
}
