import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { LucideX } from 'lucide-react';
import { useState } from 'react';
import CategoryPanel from './category-panel';

const ChunkMethodLearnMore = ({ parserId }: { parserId: string }) => {
  const [visible, setVisible] = useState(false);

  return (
    <div className={cn('hidden flex-1', 'flex flex-col')}>
      <div>
        <Button
          variant="outline"
          onClick={() => {
            setVisible(!visible);
          }}
        >
          {t('knowledgeDetails.learnMore')}
        </Button>
      </div>

      <Card
        as="article"
        className="relative flex-1 overflow-auto mt-4"
        style={{ display: visible ? 'block' : 'none' }}
      >
        <Button
          className="absolute right-2 top-2"
          variant="ghost"
          size="icon-xs"
          onClick={() => setVisible(false)}
        >
          <LucideX />
        </Button>

        <CardContent className="p-5">
          <CategoryPanel chunkMethod={parserId}></CategoryPanel>
        </CardContent>
      </Card>
    </div>
  );
};

export default ChunkMethodLearnMore;
