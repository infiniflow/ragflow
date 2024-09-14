import IndentedTree from '@/components/indented-tree/indented-tree';
import { IModalProps } from '@/interfaces/common';
import { Drawer, Flex, Progress } from 'antd';
import { useTranslation } from 'react-i18next';
import { usePendingMindMap } from './hooks';

interface IProps extends IModalProps<any> {
  data: any;
}

const MindMapDrawer = ({ data, hideModal, visible, loading }: IProps) => {
  const { t } = useTranslation();
  const percent = usePendingMindMap();
  return (
    <Drawer
      title={t('chunk.mind')}
      onClose={hideModal}
      open={visible}
      width={'40vw'}
    >
      {loading ? (
        <Flex justify="center">
          <Progress type="circle" percent={percent} size={200} />
        </Flex>
      ) : (
        <IndentedTree
          data={data}
          show
          style={{ width: '100%', height: '100%' }}
        ></IndentedTree>
      )}
    </Drawer>
  );
};

export default MindMapDrawer;
