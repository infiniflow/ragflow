import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import { useCreateMcpServer, useDeleteMcpServer, useUpdateMcpServer } from '@/hooks/mcp-server-setting-hooks';
import { IMcpServerInfo } from '@/interfaces/database/mcp-server';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

export const useAddMcpServer = () => {
  const { createMcpServer } = useCreateMcpServer();
  const { updateMcpServer } = useUpdateMcpServer();
  const {
    visible: addingMcpServerModalVisible,
    hideModal: hideAddingMcpServerModal,
    showModal: showAddingMcpServerModal,
  } = useSetModalState();

  const handleAddMcpServerOk = useCallback(
    async (serverInfo: IMcpServerInfo) => {
      let code;

      if (serverInfo.id) {
        code = await updateMcpServer(serverInfo);
      } else {
        code = await createMcpServer(serverInfo);
      }

      if (code === 0) {
        hideAddingMcpServerModal();
      }
    },
    [createMcpServer, updateMcpServer, hideAddingMcpServerModal],
  );

  return {
    addingMcpServerModalVisible,
    hideAddingMcpServerModal,
    showAddingMcpServerModal,
    handleAddMcpServerOk,
  };
};

export const useHandleDeleteMcpServer = () => {
  const { deleteMcpServer, loading } = useDeleteMcpServer();
  const showDeleteConfirm = useShowDeleteConfirm();
  const { t } = useTranslation();

  const handleDeleteMcpServer = (id: string) => () => {
    showDeleteConfirm({
      title: t('setting.mcpServerSureDelete'),
      onOk: async () => {
        const code = await deleteMcpServer({ id });
        if (code === 0) {
        }
        return;
      },
    });
  };

  return { handleDeleteMcpServer, loading };
};
