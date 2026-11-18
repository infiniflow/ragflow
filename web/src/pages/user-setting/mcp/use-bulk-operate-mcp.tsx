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

import { useOwnerMcpList } from '@/ee/pages/user-setting/mcp/use-owner-mcp-list';
import { useDeleteMcpServer } from '@/hooks/use-mcp-request';
import { IMcpServer } from '@/interfaces/database/mcp';
import { Trash2, Upload } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useExportMcp } from './use-export-mcp';

export function useBulkOperateMCP(mcpList: IMcpServer[]) {
  const { t } = useTranslation();
  const [selectedList, setSelectedList] = useState<Array<string>>([]);
  const { deleteMcpServer } = useDeleteMcpServer();
  const { handleExportMcpJson } = useExportMcp();

  const resetSelection = useCallback(() => {
    setSelectedList([]);
  }, []);

  const handleDelete = useCallback(async () => {
    const deletedIds = selectedList;
    const ret = await deleteMcpServer(deletedIds);
    if (ret.code === 0) {
      setSelectedList((list) => list.filter((id) => !deletedIds.includes(id)));
    }
  }, [deleteMcpServer, selectedList]);

  const handleSelectChange = useCallback((id: string, checked: boolean) => {
    setSelectedList((list) => {
      return checked ? [...list, id] : list.filter((item) => item !== id);
    });
  }, []);

  const ownerMcpList = useOwnerMcpList(mcpList);

  const handleSelectAll = useCallback(
    (checked: boolean) => {
      setSelectedList(() =>
        checked ? ownerMcpList.map((item) => item.id) : [],
      );
    },
    [ownerMcpList],
  );

  const selectAllChecked: boolean | 'indeterminate' =
    selectedList.length === 0
      ? false
      : selectedList.length === ownerMcpList.length
        ? true
        : 'indeterminate';

  const list = [
    {
      id: 'export',
      label: t('mcp.export'),
      icon: <Upload />,
      onClick: handleExportMcpJson(selectedList),
    },
    {
      id: 'delete',
      label: t('common.delete'),
      icon: <Trash2 />,
      onClick: handleDelete,
    },
  ];

  return {
    list,
    selectedList,
    handleSelectChange,
    handleDelete,
    handleExportMcp: handleExportMcpJson(selectedList),
    handleSelectAll,
    selectAllChecked,
    resetSelection,
  };
}

export type UseBulkOperateMCPReturnType = ReturnType<typeof useBulkOperateMCP>;
