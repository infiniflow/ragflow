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

  const handleDelete = useCallback(() => {
    deleteMcpServer(selectedList);
  }, [deleteMcpServer, selectedList]);

  const handleSelectChange = useCallback((id: string, checked: boolean) => {
    setSelectedList((list) => {
      return checked ? [...list, id] : list.filter((item) => item !== id);
    });
  }, []);

  const handleSelectAll = useCallback(
    (checked: boolean) => {
      setSelectedList(() => (checked ? mcpList.map((item) => item.id) : []));
    },
    [mcpList],
  );

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
  };
}

export type UseBulkOperateMCPReturnType = ReturnType<typeof useBulkOperateMCP>;
