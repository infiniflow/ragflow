import { Divider } from 'antd';
import { useTranslation } from 'react-i18next';

import { AppstoreAddOutlined } from '@ant-design/icons';
import AddingMcpServerModal from './add-mcp-server-modal';
import { useAddMcpServer } from './hooks';
import styles from './index.less';
import McpServerTable from './mcp-server-table';
import { useState } from 'react';
import SettingTitle from '../components/setting-title';

const UserSettingMcpServer = () => {
  const { t } = useTranslation();

  const {
    addingMcpServerModalVisible,
    hideAddingMcpServerModal,
    showAddingMcpServerModal,
    handleAddMcpServerOk,
  } = useAddMcpServer();

  const [ currentMcpServerId, setCurrentMcpServerId ] = useState('');

  const showUpdateMcpServerModal = (serverId: string) => {
    setCurrentMcpServerId(serverId);
    showAddingMcpServerModal();
  };

  return (
    <div className={styles.mcpServerWrapper}>
      <section className="w-full space-y-6">
        <SettingTitle
          title={t('setting.mcpserver')}
          description={t('setting.mcpServerSettingsDescription')}
          showRightButton
          rightButtonIcon={<AppstoreAddOutlined />}
          rightButtonTitle={t('setting.addMcpServer')}
          clickButton={showAddingMcpServerModal}
        ></SettingTitle>
        <Divider></Divider>
        <McpServerTable
          handleUpdateMcpServer={showUpdateMcpServerModal}
        >
        </McpServerTable>
      </section>
      {addingMcpServerModalVisible && (
        <AddingMcpServerModal
          visible
          hideModal={hideAddingMcpServerModal}
          currentMcpServerId={currentMcpServerId}
          onOk={handleAddMcpServerOk}
        ></AddingMcpServerModal>
      )}
    </div>
  );
};

export default UserSettingMcpServer;
