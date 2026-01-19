import { useExportMcpServer } from '@/hooks/use-mcp-request';
import { downloadJsonFile } from '@/utils/file-util';
import { useCallback } from 'react';

export function useExportMcp() {
  const { exportMcpServer } = useExportMcpServer();

  const handleExportMcpJson = useCallback(
    (ids: string[]) => async () => {
      const data = await exportMcpServer(ids);
      if (data.code === 0) {
        downloadJsonFile(data.data, `mcp.json`);
      }
    },
    [exportMcpServer],
  );

  return {
    handleExportMcpJson,
  };
}
