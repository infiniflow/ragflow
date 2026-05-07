export interface IAddLlmRequestBody {
  llm_factory: string; // Ollama
  llm_name: string;
  model_type: string;
  api_base?: string; // chat|embedding|speech2text|image2text
  api_key?: string | Record<string, any>;
  max_tokens: number;
  is_tools?: boolean;
}

export interface IDeleteLlmRequestBody {
  llm_factory: string; // Ollama
  llm_name?: string;
}
