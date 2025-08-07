import { MoreButton } from '@/components/more-button';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useFetchConversationList } from '@/hooks/use-chat-request';
import { Plus } from 'lucide-react';

function SessionCard() {
  return (
    <Card>
      <CardContent className="px-3 py-2 flex justify-between items-center group">
        xxx
        <MoreButton></MoreButton>
      </CardContent>
    </Card>
  );
}

export function Sessions() {
  const sessionList = new Array(10).fill(1);
  const {} = useFetchConversationList();

  return (
    <section className="p-6 w-[400px] max-w-[20%]">
      <div className="flex justify-between items-center mb-4">
        <span className="text-xl font-bold">Conversations</span>
        <Button variant={'ghost'}>
          <Plus></Plus>
        </Button>
      </div>
      <div className="space-y-4">
        {sessionList.map((x) => (
          <SessionCard key={x}></SessionCard>
        ))}
      </div>
    </section>
  );
}
