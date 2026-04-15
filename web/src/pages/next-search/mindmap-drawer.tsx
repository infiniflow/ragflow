import { IndentedTree } from '@/components/indented-tree/indented-tree';
import { Progress } from '@/components/ui/progress';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { usePendingMindMap } from './hooks';

interface IProps extends IModalProps<any> {
  data: any;
}

const MindMapDrawer = ({ data, hideModal, loading, visible }: IProps) => {
  const { t } = useTranslation();
  const percent = usePendingMindMap();
  return (
    <Sheet open={visible} modal={false}>
      <SheetContent
        className="top-16 p-0 flex flex-col gap-0"
        closeIcon={false}
      >
        <SheetHeader className="border-b py-2 px-4">
          <SheetTitle className="hidden"></SheetTitle>
          <div className="flex w-full justify-between items-center">
            <div className="text-text-primary font-medium text-base">
              {t('chunk.mind')}
            </div>
            <X
              className="text-text-primary cursor-pointer"
              size={16}
              onClick={() => {
                hideModal?.();
              }}
            />
          </div>
        </SheetHeader>
        <div className="flex-1 p-4 overflow-hidden">
          {loading && (
            <div className="rounded-lg w-full h-full">
              <Progress value={percent} className="h-1 flex-1 min-w-10" />
            </div>
          )}
          {!loading && (
            <div className="bg-bg-card rounded-lg w-full h-full">
              <IndentedTree data={data}></IndentedTree>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
};

export default MindMapDrawer;
