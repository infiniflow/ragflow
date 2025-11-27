import IndentedTree from '@/components/indented-tree/indented-tree';
import { Progress } from '@/components/ui/progress';
import { IModalProps } from '@/interfaces/common';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { usePendingMindMap } from './hooks';

interface IProps extends IModalProps<any> {
  data: any;
}

const MindMapDrawer = ({ data, hideModal, loading }: IProps) => {
  const { t } = useTranslation();
  const percent = usePendingMindMap();
  return (
    <div className="w-full h-full">
      <div className="flex w-full justify-between items-center mb-2">
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
      {loading && (
        <div className=" rounded-lg p-4 w-full h-full">
          <Progress value={percent} className="h-1 flex-1 min-w-10" />
        </div>
      )}
      {!loading && (
        <div className="bg-bg-card rounded-lg p-4 w-full h-full">
          <IndentedTree
            data={data}
            show
            style={{
              width: '100%',
              height: '100%',
            }}
          ></IndentedTree>
        </div>
      )}
    </div>
  );
};

export default MindMapDrawer;
