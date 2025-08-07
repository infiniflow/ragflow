import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IFlow } from '@/interfaces/database/flow';
import { formatPureDate } from '@/utils/date';
import { ChevronRight, Trash2 } from 'lucide-react';

interface IProps {
  data: IFlow;
}

export function SearchCard({ data }: IProps) {
  const { navigateToSearch } = useNavigatePage();

  return (
    <Card className="border-colors-outline-neutral-standard">
      <CardContent className="p-4 flex gap-2 items-start group">
        <div className="flex justify-between mb-4">
          <RAGFlowAvatar
            className="w-[70px] h-[70px]"
            avatar={data.avatar}
            name={data.title}
          />
        </div>
        <div className="flex flex-col gap-1">
          <section className="flex justify-between">
            <div className="text-[20px] font-bold size-7 leading-5">
              {data.title}
            </div>
            <MoreButton></MoreButton>
          </section>

          <div>An app that does things An app that does things</div>
          <section className="flex justify-between">
            <div>
              Search app
              <p className="text-sm opacity-80">
                {formatPureDate(data.update_time)}
              </p>
            </div>
            <div className="space-x-2 invisible group-hover:visible">
              <Button variant="icon" size="icon" onClick={navigateToSearch}>
                <ChevronRight className="h-6 w-6" />
              </Button>
              <Button variant="icon" size="icon">
                <Trash2 />
              </Button>
            </div>
          </section>
        </div>
      </CardContent>
    </Card>
  );
}
