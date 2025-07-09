import { useSetModalState } from '@/hooks/common-hooks';
import {
  useCreateMcpServer,
  useGetMcpServer,
  useUpdateMcpServer,
} from '@/hooks/use-mcp-request';
import { useCallback } from 'react';

export const useEditMcp = () => {
  const {
    visible: editVisible,
    hideModal: hideEditModal,
    showModal: showEditModal,
  } = useSetModalState();
  const { createMcpServer, loading } = useCreateMcpServer();
  const { data, setId, id } = useGetMcpServer();
  const { updateMcpServer } = useUpdateMcpServer();

  const handleShowModal = useCallback(
    (id?: string) => () => {
      if (id) {
        setId(id);
      }
      showEditModal();
    },
    [setId, showEditModal],
  );

  const handleOk = useCallback(
    async (values: any) => {
      if (id) {
        updateMcpServer(values);
      } else {
        createMcpServer(values);
      }
    },
    [createMcpServer, id, updateMcpServer],
  );

  return {
    editVisible,
    hideEditModal,
    showEditModal: handleShowModal,
    loading,
    createMcpServer,
    detail: data,
    handleOk,
  };
};
