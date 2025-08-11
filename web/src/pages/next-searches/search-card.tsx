import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { formatDate } from '@/utils/date';
import { ISearchAppProps } from './hooks';
import { SearchDropdown } from './search-dropdown';

interface IProps {
  data: ISearchAppProps;
}
export function SearchCard({ data }: IProps) {
  const { navigateToSearch } = useNavigatePage();

  return (
    <Card
      className="bg-bg-card  border-colors-outline-neutral-standard"
      onClick={() => {
        navigateToSearch(data?.id);
      }}
    >
      <CardContent className="p-4 flex gap-2 items-start group">
        <div className="flex justify-between mb-4">
          <RAGFlowAvatar
            className="w-[32px] h-[32px]"
            avatar={data.avatar}
            name={data.name}
          />
        </div>
        <div className="flex flex-col gap-1 flex-1">
          <section className="flex justify-between">
            <div className="text-[20px] font-bold w-80% leading-5">
              {data.name}
            </div>
            <SearchDropdown dataset={data}>
              <MoreButton></MoreButton>
            </SearchDropdown>
          </section>

          <div>{data.description}</div>
          <section className="flex justify-between">
            <div>
              Search app
              <p className="text-sm opacity-80">
                {formatDate(data.update_time)}
              </p>
            </div>
            {/* <div className="space-x-2 invisible group-hover:visible">
              <Button variant="icon" size="icon" onClick={navigateToSearch}>
                <ChevronRight className="h-6 w-6" />
              </Button>
              <Button variant="icon" size="icon">
                <Trash2 />
              </Button>
            </div> */}
          </section>
        </div>
      </CardContent>
    </Card>
  );
}
