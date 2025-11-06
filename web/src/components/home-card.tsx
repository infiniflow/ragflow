import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent } from '@/components/ui/card';
import { formatDate } from '@/utils/date';
import { ReactNode } from 'react';

interface IProps {
  data: {
    name: string;
    description?: string;
    avatar?: string;
    update_time?: string | number;
  };
  onClick?: () => void;
  moreDropdown: React.ReactNode;
  sharedBadge?: ReactNode;
  icon?: React.ReactNode;
}
export function HomeCard({
  data,
  onClick,
  moreDropdown,
  sharedBadge,
  icon,
}: IProps) {
  return (
    <Card
      onClick={() => {
        // navigateToSearch(data?.id);
        onClick?.();
      }}
    >
      <CardContent className="p-4 flex gap-2 items-start group h-full w-full hover:shadow-md">
        <div className="flex justify-between mb-4">
          <RAGFlowAvatar
            className="w-[32px] h-[32px]"
            avatar={data.avatar}
            name={data.name}
          />
        </div>
        <div className="flex flex-col justify-between gap-1 flex-1 h-full w-[calc(100%-50px)]">
          <section className="flex justify-between">
            <section className="flex flex-1 min-w-0 gap-1 items-center">
              <div className="text-base font-bold leading-snug truncate">
                {data.name}
              </div>
              {icon}
            </section>
            {moreDropdown}
          </section>

          <section className="flex flex-col gap-1 mt-1">
            <div className="whitespace-nowrap overflow-hidden text-ellipsis">
              {data.description}
            </div>
            <div className="flex justify-between items-center">
              <p className="text-sm opacity-80 whitespace-nowrap">
                {formatDate(data.update_time)}
              </p>
              {sharedBadge}
            </div>
          </section>
        </div>
      </CardContent>
    </Card>
  );
}
