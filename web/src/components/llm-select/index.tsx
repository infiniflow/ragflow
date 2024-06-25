import { Popover, Select } from 'antd';
import LlmSettingItems from '../llm-setting-items';

const LLMSelect = () => {
  const content = (
    <div>
      <LlmSettingItems handleParametersChange={() => {}}></LlmSettingItems>
    </div>
  );

  return (
    <Popover content={content} trigger="click" placement="left" arrow={false}>
      <Select style={{ width: '100%' }} dropdownStyle={{ display: 'none' }} />
    </Popover>
  );
};

export default LLMSelect;
