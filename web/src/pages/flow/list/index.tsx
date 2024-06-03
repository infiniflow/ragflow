import RenameModal from '@/components/rename-modal';
import { PlusOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import { useFetchDataOnMount, useSaveFlow } from './hooks';

const FlowList = () => {
  const {
    showFlowSettingModal,
    hideFlowSettingModal,
    flowSettingVisible,
    flowSettingLoading,

    onFlowOk,
  } = useSaveFlow();

  useFetchDataOnMount();
  return (
    <div>
      <Button
        type="primary"
        icon={<PlusOutlined />}
        onClick={showFlowSettingModal}
      >
        createKnowledgeBase
      </Button>
      <RenameModal
        visible={flowSettingVisible}
        onOk={onFlowOk}
        loading={flowSettingLoading}
        hideModal={hideFlowSettingModal}
        initialName=""
      ></RenameModal>
    </div>
  );
};

export default FlowList;
