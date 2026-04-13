export enum McpServerType {
    Sse = 'sse',
    StreamableHttp = 'streamable-http',
}

export interface IMcpServerVariable {
    key: string;
    name: string;
}

export interface IMcpServerInfo {
    id: string;
    name: string;
    url: string;
    server_type: McpServerType;
    description?: string;
    variables?: IMcpServerVariable[];
    headers: Map<string, string>;
}
