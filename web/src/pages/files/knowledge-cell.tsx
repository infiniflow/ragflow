import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';

import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { IFile } from '@/interfaces/database/file-manager';
import { Ellipsis } from 'lucide-react';
import { useCallback } from 'react';

export function KnowledgeCell({ value }: { value: IFile['kbs_info'] }) {
  const renderBadges = useCallback((list: IFile['kbs_info'] = []) => {
    return list.map((x) => (
      <Badge key={x.kb_id} variant={'secondary'}>
        {x.kb_name}
      </Badge>
    ));
  }, []);

  return Array.isArray(value) ? (
    <section className="flex gap-2 items-center">
      {renderBadges(value?.slice(0, 2))}

      {value.length > 2 && (
        <HoverCard>
          <HoverCardTrigger>
            <Button variant={'ghost'} size={'sm'}>
              <Ellipsis />
            </Button>
          </HoverCardTrigger>
          <HoverCardContent className="flex gap-2 flex-wrap">
            {renderBadges(value)}
          </HoverCardContent>
        </HoverCard>
      )}
    </section>
  ) : (
    ''
  );
}
