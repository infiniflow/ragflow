import { LlmSettingFieldItems } from '@/components/llm-setting-items/next';

export function ChatModelSettings() {
  return (
    <div className="space-y-8">
      <LlmSettingFieldItems prefix="llm_setting"></LlmSettingFieldItems>
    </div>
  );
}
