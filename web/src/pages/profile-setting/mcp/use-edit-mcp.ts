import { useSetModalState } from '@/hooks/common-hooks';
import {
  useCreateMcpServer,
  useUpdateMcpServer,
} from '@/hooks/use-mcp-request';
import { useCallback, useState } from 'react';

export const useEditMcp = () => {
  const {
    visible: editVisible,
    hideModal: hideEditModal,
    showModal: showEditModal,
  } = useSetModalState();
  const { createMcpServer, loading } = useCreateMcpServer();
  const [id, setId] = useState('');

  const { updateMcpServer, loading: updateLoading } = useUpdateMcpServer();

  const handleShowModal = useCallback(
    (id: string) => () => {
      setId(id);
      showEditModal();
    },
    [setId, showEditModal],
  );

  const handleOk = useCallback(
    async (values: any) => {
      let code;
      if (id) {
        code = await updateMcpServer({ ...values, mcp_id: id });
      } else {
        code = await createMcpServer(values);
      }
      if (code === 0) {
        hideEditModal();
      }
    },
    [createMcpServer, hideEditModal, id, updateMcpServer],
  );

  return {
    editVisible,
    hideEditModal,
    showEditModal: handleShowModal,
    loading: loading || updateLoading,
    createMcpServer,
    handleOk,
    id,
  };
};

export type UseEditMcpReturnType = ReturnType<typeof useEditMcp>;
