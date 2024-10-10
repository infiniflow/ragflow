export interface ApiKeyPostBody {
  api_key: string;
  base_url: string;
  default_model?: string;
  api_version?: string;
  group_id?: string;
}
