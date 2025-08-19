import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { formatDate } from '@/utils/date';
import { ISearchAppProps } from './hooks';
import { SearchDropdown } from './search-dropdown';

interface IProps {
  data: ISearchAppProps;
  showSearchRenameModal: (data: ISearchAppProps) => void;
}
export function SearchCard({ data, showSearchRenameModal }: IProps) {
  const { navigateToSearch } = useNavigatePage();

  return (
    <Card
      className="bg-bg-card  border-colors-outline-neutral-standard"
      onClick={() => {
        navigateToSearch(data?.id);
      }}
    >
      <CardContent className="p-4 flex gap-2 items-start group h-full">
        <div className="flex justify-between mb-4">
          <RAGFlowAvatar
            className="w-[32px] h-[32px]"
            avatar={data.avatar}
            name={data.name}
          />
        </div>
        <div className="flex flex-col justify-between gap-1 flex-1 h-full">
          <section className="flex justify-between">
            <div className="text-[20px] font-bold w-80% leading-5">
              {data.name}
            </div>
            <SearchDropdown
              dataset={data}
              showSearchRenameModal={showSearchRenameModal}
            >
              <MoreButton></MoreButton>
            </SearchDropdown>
          </section>

          <section className="flex flex-col gap-1 mt-1">
            <div>{data.description}</div>
            <div>
              <p className="text-sm opacity-80">
                {formatDate(data.update_time)}
              </p>
            </div>
          </section>
        </div>
      </CardContent>
    </Card>
  );
}
