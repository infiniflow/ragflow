import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Settings, Trash2 } from 'lucide-react';
import { useDeleteDataSource } from '../hooks';
import { IDataSorceInfo, IDataSourceBase } from '../interface';
import { delSourceModal } from './delete-source-modal';

export type IAddedSourceCardProps = IDataSorceInfo & {
  list: IDataSourceBase[];
};
export const AddedSourceCard = (props: IAddedSourceCardProps) => {
  const { list, name, icon } = props;
  const { handleDelete } = useDeleteDataSource();
  const { navigateToDataSourceDetail } = useNavigatePage();
  const toDetail = (id: string) => {
    navigateToDataSourceDetail(id);
  };
  return (
    <Card className="bg-transparent border border-border-button px-5 pt-[10px] pb-5 rounded-md">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 p-0 pb-3">
        {/* <Users className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
        <CardTitle className="text-base items-center flex gap-1 font-normal">
          {icon}
          {name}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-2 flex flex-col gap-2">
        {list.map((item) => (
          <div
            key={item.id}
            className="flex flex-row items-center justify-between rounded-md bg-bg-card px-[10px] py-4"
          >
            <div className="text-sm text-text-secondary ">{item.name}</div>
            <div className="text-sm text-text-secondary  flex gap-2">
              <Settings
                className="cursor-pointer"
                size={14}
                onClick={() => {
                  toDetail(item.id);
                }}
              />
              {/* <ConfirmDeleteDialog onOk={() => handleDelete(item)}> */}
              <Trash2
                className="cursor-pointer"
                size={14}
                onClick={() =>
                  delSourceModal({
                    data: item,
                    onOk: () => {
                      handleDelete(item);
                    },
                  })
                }
              />
              {/* </ConfirmDeleteDialog> */}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
};
