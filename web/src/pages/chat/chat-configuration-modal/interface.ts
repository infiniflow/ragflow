import { FormInstance } from 'antd';

export interface ISegmentedContentProps {
  show: boolean;
  form: FormInstance;
}

export interface IVariable {
  temperature: number;
  top_p: number;
  frequency_penalty: number;
  presence_penalty: number;
  max_tokens: number;
}
