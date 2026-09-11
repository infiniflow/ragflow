/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import {
  useAddTenantUser,
  useAgreeTenant,
  useDeleteTenantUser,
  useFetchUserInfo,
} from '@/hooks/use-user-setting-request';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

export const useAddUser = () => {
  const { addTenantUser } = useAddTenantUser();
  const {
    visible: addingTenantModalVisible,
    hideModal: hideAddingTenantModal,
    showModal: showAddingTenantModal,
  } = useSetModalState();

  const handleAddUserOk = useCallback(
    async (email: string) => {
      const code = await addTenantUser(email);
      if (code === 0) {
        hideAddingTenantModal();
      }
    },
    [addTenantUser, hideAddingTenantModal],
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
  const { t } = useTranslation();

  const handleDeleteTenantUser = (userId: string) => () => {
    showDeleteConfirm({
      title: t('setting.sureDelete'),
      onOk: async () => {
        const code = await deleteTenantUser({ userId });
        if (code === 0) {
        }
        return;
      },
    });
  };

  return { handleDeleteTenantUser, deleteTenantUser, loading };
};

export const useHandleAgreeTenant = () => {
  const { agreeTenant } = useAgreeTenant();
  const { deleteTenantUser } = useDeleteTenantUser();
  const { data: user } = useFetchUserInfo();

  const handleAgree = (tenantId: string, isAgree: boolean) => () => {
    if (isAgree) {
      agreeTenant(tenantId);
    } else {
      deleteTenantUser({ tenantId, userId: user.id });
    }
  };

  return { handleAgree };
};

export const useHandleQuitUser = () => {
  const { deleteTenantUser, loading } = useDeleteTenantUser();
  const showDeleteConfirm = useShowDeleteConfirm();
  const { t } = useTranslation();

  const handleQuitTenantUser = (userId: string, tenantId: string) => () => {
    showDeleteConfirm({
      title: t('setting.sureQuit'),
      onOk: async () => {
        deleteTenantUser({ userId, tenantId });
      },
    });
  };

  return { handleQuitTenantUser, loading };
};
