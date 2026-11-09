export interface ISetLangfuseConfigRequestBody {
  secret_key: string;
  public_key: string;
  host: string;
}

export interface ISetFutureAGIConfigRequestBody {
  api_key: string;
  secret_key: string;
  project_name: string;
  endpoint: string;
  redact: boolean;
}
