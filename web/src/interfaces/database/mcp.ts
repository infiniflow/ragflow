export interface IMcpServer {
  create_date: string;
  description: null;
  id: string;
  name: string;
  server_type: string;
  update_date: string;
  url: string;
  variables: Record<string, any> & { tools?: IMCPToolObject };
}

export type IMCPToolObject = Record<string, Omit<IMCPTool, 'name'>>;

export type IMCPToolRecord = Record<string, IMCPTool>;

export interface IMcpServerListResponse {
  mcp_servers: IMcpServer[];
  total: number;
}

export interface IMCPTool {
  annotations: null;
  description: string;
  enabled: boolean;
  inputSchema: InputSchema;
  name: string;
}

interface InputSchema {
  properties: Properties;
  required: string[];
  title: string;
  type: string;
}

interface Properties {
  symbol: ISymbol;
}

interface ISymbol {
  title: string;
  type: string;
}

export interface IExportedMcpServers {
  mcpServers: McpServers;
}

interface McpServers {
  fetch_2: IExportedMcpServer;
  github_1: IExportedMcpServer;
}

export interface IExportedMcpServer {
  authorization_token: string;
  name: string;
  tool_configuration: Record<string, any>;
  type: string;
  url: string;
}
