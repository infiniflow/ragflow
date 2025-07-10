export interface ITestMcpRequestBody {
  server_type: string;
  url: string;
  headers?: Record<string, any>;
  variables?: Record<string, any>;
  timeout?: number;
}
