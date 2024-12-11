import { useFetchInputElements } from '@/hooks/flow-hooks';
import { IModalProps } from '@/interfaces/common';
import { CloseOutlined } from '@ant-design/icons';
import { Drawer } from 'antd';
import { useTranslation } from 'react-i18next';
import DebugContent from '../../debug-content';

interface IProps {
  componentId?: string;
}

const SingleDebugDrawer = ({
  componentId,
  visible,
  hideModal,
}: IModalProps<any> & IProps) => {
  const { t } = useTranslation();
  const { data } = useFetchInputElements(componentId);

  return (
    <Drawer
      title={
        <div className="flex justify-between">
          {t('flow.testRun')}
          <CloseOutlined onClick={hideModal} />
        </div>
      }
      width={'100%'}
      onClose={hideModal}
      open={visible}
      getContainer={false}
      mask={false}
      placement={'bottom'}
      height={'95%'}
      closeIcon={null}
    >
      <DebugContent parameters={data}></DebugContent>
    </Drawer>
  );
};

export default SingleDebugDrawer;
