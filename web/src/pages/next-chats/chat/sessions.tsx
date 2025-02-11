import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { EllipsisVertical, Plus } from 'lucide-react';

function SessionCard() {
  return (
    <Card className="bg-colors-background-inverse-weak border-colors-outline-neutral-standard">
      <CardContent className="px-3 py-2 flex justify-between items-center">
        xxx
        <Button variant={'icon'} size={'icon'}>
          <EllipsisVertical />
        </Button>
      </CardContent>
    </Card>
  );
}

export function Sessions() {
  const sessionList = new Array(10).fill(1);

  return (
    <section className="p-6 w-[400px] max-w-[20%]">
      <div className="flex justify-between items-center mb-4">
        <span className="text-colors-text-neutral-strong text-2xl font-bold">
          Sessions
        </span>
        <Button variant={'icon'} size={'icon'}>
          <Plus></Plus>
        </Button>
      </div>
      <div className="space-y-4">
        {sessionList.map((x) => (
          <SessionCard key={x.id}></SessionCard>
        ))}
      </div>
    </section>
  );
}
