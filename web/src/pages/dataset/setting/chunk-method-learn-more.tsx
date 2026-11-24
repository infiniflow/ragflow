import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { FolderBookmark } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import CategoryPanel from './category-panel';

const ChunkMethodLearnMore = ({ parserId }: { parserId: string }) => {
  const { t } = useTranslation();

  return (
    <Sheet modal={false}>
      <SheetTrigger asChild>
        <Button type="button" variant="outline">
          <FolderBookmark />
          {t('knowledgeDetails.learnMore')}
        </Button>
      </SheetTrigger>
      <SheetContent
        side="right"
        className="flex flex-col"
        aria-describedby={undefined}
      >
        <SheetHeader>
          <SheetTitle>{t('knowledgeDetails.builtInParserDetails')}</SheetTitle>
        </SheetHeader>
        <div className="flex-1 min-h-0 overflow-y-auto">
          <CategoryPanel chunkMethod={parserId} />
        </div>
      </SheetContent>
    </Sheet>
  );
};

export default ChunkMethodLearnMore;
