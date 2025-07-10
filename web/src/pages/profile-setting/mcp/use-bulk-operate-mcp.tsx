import { useDeleteMcpServer } from '@/hooks/use-mcp-request';
import { Trash2, Upload } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

export function useBulkOperateMCP() {
  const { t } = useTranslation();
  const [selectedList, setSelectedList] = useState<Array<string>>([]);
  const { deleteMcpServer } = useDeleteMcpServer();

  const handleEnableClick = useCallback(() => {}, []);

  const handleDelete = useCallback(() => {
    deleteMcpServer(selectedList);
  }, [deleteMcpServer, selectedList]);

  const handleSelectChange = useCallback((id: string, checked: boolean) => {
    setSelectedList((list) => {
      return checked ? [...list, id] : list.filter((item) => item !== id);
    });
  }, []);

  const list = [
    {
      id: 'export',
      label: t('mcp.export'),
      icon: <Upload />,
      onClick: handleEnableClick,
    },
    {
      id: 'delete',
      label: t('common.delete'),
      icon: <Trash2 />,
      onClick: handleDelete,
    },
  ];

  return { list, selectedList, handleSelectChange };
}

export type UseBulkOperateMCPReturnType = ReturnType<typeof useBulkOperateMCP>;
