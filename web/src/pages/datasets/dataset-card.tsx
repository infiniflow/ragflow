import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { formatDate } from '@/utils/date';
import { Ellipsis } from 'lucide-react';
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
      className="bg-colors-background-inverse-weak flex-1"
      onClick={navigateToDataset(dataset.id)}
    >
      <CardContent className="p-4">
        <section className="flex justify-between mb-4">
          <div className="flex  gap-2">
            <Avatar className="w-[70px] h-[70px] rounded-lg">
              <AvatarImage src={dataset.avatar} />
              <AvatarFallback className="rounded-lg">CN</AvatarFallback>
            </Avatar>
            {owner && <Badge className="h-5">{owner}</Badge>}
          </div>
          <DatasetDropdown
            showDatasetRenameModal={showDatasetRenameModal}
            dataset={dataset}
          >
            <Button variant="ghost" size="icon">
              <Ellipsis />
            </Button>
          </DatasetDropdown>
        </section>
        <div className="flex justify-between items-end">
          <div>
            <h3 className="text-lg font-semibold mb-2 line-clamp-1">
              {dataset.name}
            </h3>
            <p className="text-sm opacity-80">{dataset.doc_num} files</p>
            <p className="text-sm opacity-80">
              Created {formatDate(dataset.update_time)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
