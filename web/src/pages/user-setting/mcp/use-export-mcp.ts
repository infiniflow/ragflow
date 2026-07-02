import { useExportMcpServer } from '@/hooks/use-mcp-request';
import { IMcpServer } from '@/interfaces/database/mcp';
import { downloadJsonFile } from '@/utils/file-util';
import { useCallback } from 'react';

export function useExportMcp(mcp: IMcpServer) {
  const { exportMcpServer } = useExportMcpServer();

  const handleExportMcpJson = useCallback(
    (ids: string[]) => async () => {
      const data = await exportMcpServer(ids);
      if (data.code === 0 && mcp) {
        downloadJsonFile(data.data, `${mcp.name || 'mcp'}.json`);
      }
    },
    [exportMcpServer, mcp],
  );

  return {
    handleExportMcpJson,
  };
}
