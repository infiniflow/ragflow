import { ReactComponent as ChatConfigurationAtom } from '@/assets/svg/chat-configuration-atom.svg';
import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { Divider, Flex, Form, Modal, Segmented } from 'antd';
import { SegmentedValue } from 'antd/es/segmented';
import omit from 'lodash/omit';
import { useRef, useState } from 'react';
import AssistantSetting from './assistant-setting';
import ModelSetting from './model-setting';
import PromptEngine from './prompt-engine';

import { useSetDialog } from '../hooks';
import { variableEnabledFieldMap } from './constants';
import styles from './index.less';

enum ConfigurationSegmented {
  AssistantSetting = 'Assistant Setting',
  PromptEngine = 'Prompt Engine',
  ModelSetting = 'Model Setting',
}

const segmentedMap = {
  [ConfigurationSegmented.AssistantSetting]: AssistantSetting,
  [ConfigurationSegmented.ModelSetting]: ModelSetting,
  [ConfigurationSegmented.PromptEngine]: PromptEngine,
};

const layout = {
  labelCol: { span: 6 },
  wrapperCol: { span: 18 },
};

const validateMessages = {
  required: '${label} is required!',
  types: {
    email: '${label} is not a valid email!',
    number: '${label} is not a valid number!',
  },
  number: {
    range: '${label} must be between ${min} and ${max}',
  },
};

const ChatConfigurationModal = ({
  visible,
  hideModal,
}: IModalManagerChildrenProps) => {
  const [form] = Form.useForm();
  const [value, setValue] = useState<ConfigurationSegmented>(
    ConfigurationSegmented.AssistantSetting,
  );
  const promptEngineRef = useRef(null);

  const setDialog = useSetDialog();

  const handleOk = async () => {
    const values = await form.validateFields();
    const nextValues: any = omit(values, Object.keys(variableEnabledFieldMap));
    const finalValues = {
      ...nextValues,
      prompt_config: {
        ...nextValues.prompt_config,
        parameters: promptEngineRef.current,
      },
    };
    console.info(promptEngineRef.current);
    console.info(nextValues);
    console.info(finalValues);
    setDialog(finalValues);
  };

  const handleCancel = () => {
    hideModal();
  };

  const handleSegmentedChange = (val: SegmentedValue) => {
    setValue(val as ConfigurationSegmented);
  };

  const title = (
    <Flex gap={16}>
      <ChatConfigurationAtom></ChatConfigurationAtom>
      <div>
        <b>Chat Configuration</b>
        <div className={styles.chatConfigurationDescription}>
          Here, dress up a dedicated assistant for your special knowledge bases!
          💕
        </div>
      </div>
    </Flex>
  );

  return (
    <Modal
      title={title}
      width={688}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
    >
      <Segmented
        size={'large'}
        value={value}
        onChange={handleSegmentedChange}
        options={Object.values(ConfigurationSegmented)}
        block
      />
      <Divider></Divider>
      <Form
        {...layout}
        name="nest-messages"
        form={form}
        style={{ maxWidth: 600 }}
        validateMessages={validateMessages}
        colon={false}
      >
        {Object.entries(segmentedMap).map(([key, Element]) => (
          <Element
            key={key}
            show={key === value}
            form={form}
            {...(key === ConfigurationSegmented.PromptEngine
              ? { ref: promptEngineRef }
              : {})}
          ></Element>
        ))}
      </Form>
    </Modal>
  );
};

export default ChatConfigurationModal;
