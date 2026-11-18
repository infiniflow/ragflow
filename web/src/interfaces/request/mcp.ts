import { IExportedMcpServer } from '@/interfaces/database/mcp';
import { IPaginationRequestBody } from './base';

export interface IMcpServerListRequestBody extends IPaginationRequestBody {
  tenant_id?: string;
}

export interface ITestMcpRequestBody {
  mcp_id?: string;
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
