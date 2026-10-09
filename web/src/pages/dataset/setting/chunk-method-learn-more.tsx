import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { LucideX } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CategoryPanel from './category-panel';

const ChunkMethodLearnMore = ({ parserId }: { parserId: string }) => {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);

  const toggleVisible = useCallback(() => {
    setVisible((v) => !v);
  }, []);

  const hidePanel = useCallback(() => {
    setVisible(false);
  }, []);

  return (
    <div className="flex flex-1 flex-col">
      <div>
        <Button variant="outline" onClick={toggleVisible}>
          {t('knowledgeDetails.learnMore')}
        </Button>
      </div>

      {visible && (
        <Card as="article" className="relative flex-1 overflow-auto mt-4">
          <Button
            className="absolute right-2 top-2"
            variant="ghost"
            size="icon-xs"
            onClick={hidePanel}
          >
            <LucideX />
          </Button>

          <CardContent className="p-5">
            <CategoryPanel chunkMethod={parserId}></CategoryPanel>
          </CardContent>
        </Card>
      )}
    </div>
  );
};

export default ChunkMethodLearnMore;
