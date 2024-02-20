export interface PromptConfig {
  empty_response: string;
  parameters: Parameter[];
  prologue: string;
  system: string;
}

export interface Parameter {
  key: string;
  optional: boolean;
}

export interface LlmSetting {
  Creative: Variable;
  Custom: Variable;
  Evenly: Variable;
  Precise: Variable;
}

export interface Variable {
  frequency_penalty: number;
  max_tokens: number;
  presence_penalty: number;
  temperature: number;
  top_p: number;
}

export interface IDialog {
  create_date: string;
  create_time: number;
  description: string;
  icon: string;
  id: string;
  kb_ids: string[];
  kb_names: string[];
  language: string;
  llm_id: string;
  llm_setting: LlmSetting;
  llm_setting_type: string;
  name: string;
  prompt_config: PromptConfig;
  prompt_type: string;
  status: string;
  tenant_id: string;
  update_date: string;
  update_time: number;
}
