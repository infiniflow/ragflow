export interface ILangfuseConfig {
  secret_key: string;
  public_key: string;
  host: string;
  project_id: string;
  project_name: string;
}

export interface IFutureAGIConfig {
  api_key: string;
  secret_key: string;
  project_name: string;
  endpoint: string;
  redact: boolean;
}
