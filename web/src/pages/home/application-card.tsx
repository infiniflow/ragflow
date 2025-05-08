import { MoreButton } from '@/components/more-button';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Card, CardContent } from '@/components/ui/card';
import { formatDate } from '@/utils/date';
import { ChevronRight } from 'lucide-react';

type ApplicationCardProps = {
  app: {
    avatar?: string;
    title: string;
    update_time: number;
  };
};

export function ApplicationCard({ app }: ApplicationCardProps) {
  return (
    <Card className="w-[264px]">
      <CardContent className="p-2.5  group flex justify-between">
        <div className="flex items-center gap-2.5">
          <Avatar className="size-14 rounded-lg">
            <AvatarImage src={app.avatar === null ? '' : app.avatar} />
            <AvatarFallback className="rounded-lg">CN</AvatarFallback>
          </Avatar>
          <div className="flex-1">
            <h3 className="text-sm font-normal line-clamp-1 mb-1">
              {app.title}
            </h3>
            <p className="text-xs font-normal text-text-sub-title">
              {formatDate(app.update_time)}
            </p>
          </div>
        </div>

        <MoreButton className=""></MoreButton>
      </CardContent>
    </Card>
  );
}

export type SeeAllAppCardProps = {
  click(): void;
};

export function SeeAllAppCard({ click }: SeeAllAppCardProps) {
  return (
    <Card className="w-64 min-h-[76px]" onClick={click}>
      <CardContent className="p-2.5 pt-1 w-full h-full flex items-center justify-center gap-1.5 text-text-sub-title">
        See All <ChevronRight className="size-4" />
      </CardContent>
    </Card>
  );
}
