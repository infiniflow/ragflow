import message from '@/components/ui/message';
import { FileMimeType } from '@/constants/common';
import { useSetModalState } from '@/hooks/common-hooks';
import { useImportMcpServer } from '@/hooks/use-mcp-request';
import { isEmpty } from 'lodash';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

const ServerEntrySchema = z.object({
  authorization_token: z.string().optional(),
  name: z.string().optional(),
  tool_configuration: z.object({}).passthrough().optional(),
  type: z.string(),
  url: z.string().url(),
});

const McpConfigSchema = z.object({
  mcpServers: z.record(ServerEntrySchema),
});

export const useImportMcp = () => {
  const {
    visible: importVisible,
    hideModal: hideImportModal,
    showModal: showImportModal,
  } = useSetModalState();
  const { t } = useTranslation();
  const { importMcpServer, loading } = useImportMcpServer();

  const onImportOk = useCallback(
    async ({ fileList }: { fileList: File[] }) => {
      if (fileList.length === 0) {
        return;
      }

      // Folder uploads may include non-JSON files; only process JSON ones.
      const jsonFiles = fileList.filter(
        (file) =>
          file.type === FileMimeType.Json ||
          file.name.toLowerCase().endsWith('.json'),
      );
      if (jsonFiles.length === 0) {
        message.error(t('flow.jsonUploadTypeErrorMessage'));
        return;
      }

      const errorMessage = t('flow.jsonUploadContentErrorMessage');
      const mergedMcpServers: Record<string, any> = {};

      for (const file of jsonFiles) {
        const mcpStr = await file.text();
        let mcp: { mcpServers?: Record<string, any> };
        try {
          mcp = JSON.parse(mcpStr);
        } catch (error) {
          console.error(error);
          message.error(errorMessage);
          return;
        }

        try {
          McpConfigSchema.parse(mcp);
        } catch (error) {
          console.error(error);
          message.error('Incorrect data format');
          return;
        }

        if (!mcpStr || isEmpty(mcp)) {
          message.error(errorMessage);
          return;
        }

        Object.assign(mergedMcpServers, mcp.mcpServers);
      }

      if (!isEmpty(mergedMcpServers)) {
        const ret = await importMcpServer({ mcpServers: mergedMcpServers });
        if (ret.code === 0) {
          hideImportModal();
        }
      }
    },
    [hideImportModal, importMcpServer, t],
  );

  return {
    importVisible,
    showImportModal,
    hideImportModal,
    onImportOk,
    loading,
  };
};
