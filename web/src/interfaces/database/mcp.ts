export interface IMcpServer {
  create_date: string;
  description: null;
  id: string;
  name: string;
  server_type: string;
  update_date: string;
  url: string;
  variables: Record<string, any>;
}

export interface IMcpServerListResponse {
  mcp_servers: IMcpServer[];
  total: number;
}
