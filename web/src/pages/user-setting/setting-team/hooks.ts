import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import {
  useAddTenantUser,
  useAgreeTenant,
  useDeleteTenantUser,
} from '@/hooks/user-setting-hooks';
import { useCallback } from 'react';

export const useAddUser = () => {
  const { addTenantUser } = useAddTenantUser();
  const {
    visible: addingTenantModalVisible,
    hideModal: hideAddingTenantModal,
    showModal: showAddingTenantModal,
  } = useSetModalState();

  const handleAddUserOk = useCallback(
    (email: string) => {
      addTenantUser(email);
    },
    [addTenantUser],
  );

  return {
    addingTenantModalVisible,
    hideAddingTenantModal,
    showAddingTenantModal,
    handleAddUserOk,
  };
};

export const useHandleDeleteUser = () => {
  const { deleteTenantUser, loading } = useDeleteTenantUser();
  const showDeleteConfirm = useShowDeleteConfirm();

  const handleDeleteTenantUser = (userId: string) => () => {
    showDeleteConfirm({
      onOk: async () => {
        const retcode = await deleteTenantUser(userId);
        if (retcode === 0) {
        }
        return;
      },
    });
  };

  return { handleDeleteTenantUser, loading };
};

export const useHandleAgreeTenant = () => {
  const { agreeTenant, loading } = useAgreeTenant();
  const showDeleteConfirm = useShowDeleteConfirm();

  const handleDeleteTenantUser = (userId: string) => () => {
    showDeleteConfirm({
      onOk: async () => {
        const retcode = await deleteTenantUser(userId);
        if (retcode === 0) {
        }
        return;
      },
    });
  };

  return { handleDeleteTenantUser, loading };
};
