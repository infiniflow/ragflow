import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { CardSkeleton } from '@/components/ui/skeleton';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { formatDate } from '@/utils/date';
import { ChevronRight, Trash2 } from 'lucide-react';

export function Datasets() {
  const { navigateToDatasetList, navigateToDataset } = useNavigatePage();
  const { list, loading } = useFetchKnowledgeList();

  return (
    <section>
      <h2 className="text-2xl font-bold mb-6">Datasets</h2>
      <div className="flex gap-6">
        {loading ? (
          <div className="flex-1">
            <CardSkeleton />
          </div>
        ) : (
          <div className="flex gap-4 flex-1">
            {list.slice(0, 3).map((dataset) => (
              <Card
                key={dataset.id}
                className="bg-colors-background-inverse-weak flex-1 border-colors-outline-neutral-standard max-w-96"
              >
                <CardContent className="p-4">
                  <div className="flex justify-between mb-4">
                    {dataset.avatar ? (
                      <div
                        className="w-[70px] h-[70px] rounded-xl bg-cover"
                        style={{ backgroundImage: `url(${dataset.avatar})` }}
                      />
                    ) : (
                      <Avatar>
                        <AvatarImage src="https://github.com/shadcn.png" />
                        <AvatarFallback>CN</AvatarFallback>
                      </Avatar>
                    )}
                    <Button variant="ghost" size="icon">
                      <Trash2 />
                    </Button>
                  </div>
                  <div className="flex justify-between items-end">
                    <div>
                      <h3 className="text-lg font-semibold mb-2">
                        {dataset.name}
                      </h3>
                      <div className="text-sm opacity-80">
                        {dataset.doc_num} files
                      </div>
                      <p className="text-sm opacity-80">
                        Created {formatDate(dataset.update_time)}
                      </p>
                    </div>
                    <Button
                      variant="icon"
                      size="icon"
                      onClick={navigateToDataset(dataset.id)}
                    >
                      <ChevronRight className="h-6 w-6" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
        <Button
          className="h-auto "
          variant={'tertiary'}
          onClick={navigateToDatasetList}
        >
          See all
        </Button>
      </div>
    </section>
  );
}
