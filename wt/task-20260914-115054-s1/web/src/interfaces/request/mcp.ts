import { IExportedMcpServer } from '@/interfaces/database/mcp';

export interface ITestMcpRequestBody {
  server_type: string;
  url: string;
  headers?: Record<string, any>;
  variables?: Record<string, any>;
  timeout?: number;
}

export interface IImportMcpServersRequestBody {
  mcpServers: Record<
    string,
    Pick<IExportedMcpServer, 'type' | 'url' | 'authorization_token'>
  >;
}
