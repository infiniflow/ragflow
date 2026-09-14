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
