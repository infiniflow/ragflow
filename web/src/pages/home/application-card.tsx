import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Card, CardContent } from '@/components/ui/card';
import { formatDate } from '@/utils/date';

type ApplicationCardProps = {
  app: {
    avatar?: string;
    title: string;
    update_time: number;
  };
};

export function ApplicationCard({ app }: ApplicationCardProps) {
  return (
    <Card className="bg-colors-background-inverse-weak border-colors-outline-neutral-standard w-64">
      <CardContent className="p-4 flex items-center gap-6">
        <Avatar className="size-14 rounded-lg">
          <AvatarImage src={app.avatar === null ? '' : app.avatar} />
          <AvatarFallback className="rounded-lg">CN</AvatarFallback>
        </Avatar>
        <div className="flex-1">
          <h3 className="text-lg font-semibold line-clamp-1 mb-1">
            {app.title}
          </h3>
          <p className="text-sm opacity-80">{formatDate(app.update_time)}</p>
        </div>
      </CardContent>
    </Card>
  );
}
