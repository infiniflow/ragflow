import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IDialog } from '@/interfaces/database/chat';
import { formatPureDate } from '@/utils/date';
import { ChevronRight, Trash2 } from 'lucide-react';

interface IProps {
  data: IDialog;
}

export function ChatCard({ data }: IProps) {
  const { navigateToChat } = useNavigatePage();

  return (
    <Card className="bg-colors-background-inverse-weak  border-colors-outline-neutral-standard">
      <CardContent className="p-4">
        <div className="flex justify-between mb-4">
          {data.icon ? (
            <div
              className="w-[70px] h-[70px] rounded-xl bg-cover"
              style={{ backgroundImage: `url(${data.icon})` }}
            />
          ) : (
            <Avatar className="w-[70px] h-[70px]">
              <AvatarImage src="https://github.com/shadcn.png" />
              <AvatarFallback>CN</AvatarFallback>
            </Avatar>
          )}
        </div>
        <h3 className="text-xl font-bold mb-2">{data.name}</h3>
        <p>An app that does things An app that does things</p>
        <section className="flex justify-between pt-3">
          <div>
            Search app
            <p className="text-sm opacity-80">
              {formatPureDate(data.update_time)}
            </p>
          </div>
          <div className="space-x-2">
            <Button variant="icon" size="icon" onClick={navigateToChat}>
              <ChevronRight className="h-6 w-6" />
            </Button>
            <Button variant="icon" size="icon">
              <Trash2 />
            </Button>
          </div>
        </section>
      </CardContent>
    </Card>
  );
}
