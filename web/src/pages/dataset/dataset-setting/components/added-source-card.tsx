import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import {
  IDataSorceInfo,
  IDataSourceBase,
} from '@/pages/user-setting/data-source/interface';
import { Check } from 'lucide-react';
import { useMemo } from 'react';

export type IAddedSourceCardProps = IDataSorceInfo & {
  filterString: string;
  list: IDataSourceBase[];
  selectedList: IDataSourceBase[];
  setSelectedList: (list: IDataSourceBase[]) => void;
};
export const AddedSourceCard = (props: IAddedSourceCardProps) => {
  const {
    list: originList,
    name,
    icon,
    filterString,
    selectedList,
    setSelectedList,
  } = props;

  const list = useMemo(() => {
    return originList.map((item) => {
      const checked = selectedList?.some((i) => i.id === item.id) || false;
      return {
        ...item,
        checked: checked,
      };
    });
  }, [originList, selectedList]);

  const filterList = useMemo(
    () => list.filter((item) => item.name.indexOf(filterString) > -1),
    [filterString, list],
  );

  // const { navigateToDataSourceDetail } = useNavigatePage();
  // const toDetail = (id: string) => {
  //   navigateToDataSourceDetail(id);
  // };

  const onCheck = (item: IDataSourceBase & { checked: boolean }) => {
    if (item.checked) {
      setSelectedList(selectedList.filter((i) => i.id !== item.id));
    } else {
      setSelectedList([...(selectedList || []), item]);
    }
  };
  return (
    <>
      {filterList.length > 0 && (
        <Card className="bg-transparent border border-border-button px-5 pt-[10px] pb-5 rounded-md">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 p-0 pb-3">
            {/* <Users className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
            <CardTitle className="text-base flex gap-1 font-normal">
              {icon}
              {name}
            </CardTitle>
          </CardHeader>
          <CardContent className="p-2 flex flex-col gap-2">
            {filterList.map((item) => (
              <div
                key={item.id}
                className={cn(
                  'flex flex-row items-center justify-between rounded-md bg-bg-card px-2 py-1 cursor-pointer',
                  // { hidden: item.name.indexOf(filterString) <= -1 },
                )}
                onClick={() => {
                  console.log('item--->', item);
                  // toDetail(item.id);
                  onCheck(item);
                }}
              >
                <div className="text-sm text-text-secondary ">{item.name}</div>
                <div className="text-sm text-text-secondary  flex gap-2">
                  {item.checked && (
                    <Check
                      className="cursor-pointer"
                      size={14}
                      // onClick={() => {
                      //   toDetail(item.id);
                      // }}
                    />
                  )}
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      )}
    </>
  );
};
