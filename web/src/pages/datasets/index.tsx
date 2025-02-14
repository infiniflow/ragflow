import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useInfiniteFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { formatDate } from '@/utils/date';
import { ChevronRight, Plus, Trash2 } from 'lucide-react';
import { useMemo } from 'react';
import { DatasetCreatingDialog } from './dataset-creating-dialog';
import { useSaveKnowledge } from './hooks';

export default function Datasets() {
  const {
    visible,
    hideModal,
    showModal,
    onCreateOk,
    loading: creatingLoading,
  } = useSaveKnowledge();
  const { navigateToDataset } = useNavigatePage();

  const {
    fetchNextPage,
    data,
    hasNextPage,
    searchString,
    handleInputChange,
    loading,
  } = useInfiniteFetchKnowledgeList();

  const nextList: IKnowledge[] = useMemo(() => {
    const list =
      data?.pages?.flatMap((x) => (Array.isArray(x.kbs) ? x.kbs : [])) ?? [];
    return list;
  }, [data?.pages]);

  const total = useMemo(() => {
    return data?.pages.at(-1).total ?? 0;
  }, [data?.pages]);

  return (
    <section className="p-8 text-foreground">
      <ListFilterBar title="Datasets" showDialog={showModal}>
        <Plus className="mr-2 h-4 w-4" />
        Create dataset
      </ListFilterBar>
      <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8">
        {nextList.map((dataset) => (
          <Card
            key={dataset.id}
            className="bg-colors-background-inverse-weak flex-1"
          >
            <CardContent className="p-4">
              <div className="flex justify-between mb-4">
                <Avatar className="w-[70px] h-[70px] rounded-lg">
                  <AvatarImage src={dataset.avatar} />
                  <AvatarFallback className="rounded-lg">CN</AvatarFallback>
                </Avatar>
                <ConfirmDeleteDialog>
                  <Button variant="ghost" size="icon">
                    <Trash2 />
                  </Button>
                </ConfirmDeleteDialog>
              </div>
              <div className="flex justify-between items-end">
                <div>
                  <h3 className="text-lg font-semibold mb-2">{dataset.name}</h3>
                  <p className="text-sm opacity-80">{dataset.doc_num} files</p>
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
      {visible && (
        <DatasetCreatingDialog
          hideModal={hideModal}
          onOk={onCreateOk}
          loading={creatingLoading}
        ></DatasetCreatingDialog>
      )}
    </section>
  );
}
