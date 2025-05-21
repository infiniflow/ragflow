export enum McpServerType {
    Sse = 'sse',
    StreamableHttp = 'streamable-http',
}

export interface IMcpServerInfo {
    id: string;
    name: string;
    url: string;
    server_type: McpServerType;
    description?: string;
    headers: Map<string, string>;
}
