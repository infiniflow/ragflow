import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent } from '@/components/ui/card';
import { formatDate } from '@/utils/date';
import { ChevronRight } from 'lucide-react';

type ApplicationCardProps = {
  app: {
    avatar?: string;
    title: string;
    update_time: number;
  };
  onClick?(): void;
  moreDropdown: React.ReactNode;
};

export function ApplicationCard({
  app,
  onClick,
  moreDropdown,
}: ApplicationCardProps) {
  return (
    <Card className="w-[264px]" onClick={onClick}>
      <CardContent className="p-2.5  group flex justify-between w-full">
        <div className="flex items-center gap-2.5 w-full">
          <RAGFlowAvatar
            className="size-14 rounded-lg"
            avatar={app.avatar}
            name={app.title || 'CN'}
          ></RAGFlowAvatar>
          <div className="flex-1">
            <h3 className="text-sm font-normal line-clamp-1 mb-1 text-ellipsis w-[160px] overflow-hidden">
              {app.title}
            </h3>
            <p className="text-xs font-normal text-text-secondary">
              {formatDate(app.update_time)}
            </p>
          </div>
        </div>
        {moreDropdown}
      </CardContent>
    </Card>
  );
}

export type SeeAllAppCardProps = {
  click(): void;
};

export function SeeAllAppCard({ click }: SeeAllAppCardProps) {
  return (
    <Card className="w-full min-h-[76px] cursor-pointer" onClick={click}>
      <CardContent className="p-2.5 pt-1 w-full h-full flex items-center justify-center gap-1.5 text-text-secondary">
        See All <ChevronRight className="size-4" />
      </CardContent>
    </Card>
  );
}
