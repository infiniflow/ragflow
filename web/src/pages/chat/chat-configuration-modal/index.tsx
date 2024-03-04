import { ReactComponent as ChatConfigurationAtom } from '@/assets/svg/chat-configuration-atom.svg';
import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { Divider, Flex, Form, Modal, Segmented, UploadFile } from 'antd';
import { SegmentedValue } from 'antd/es/segmented';
import omit from 'lodash/omit';
import { useEffect, useRef, useState } from 'react';
import AssistantSetting from './assistant-setting';
import ModelSetting from './model-setting';
import PromptEngine from './prompt-engine';

import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { variableEnabledFieldMap } from '../constants';
import { useFetchDialog, useResetCurrentDialog, useSetDialog } from '../hooks';
import { IPromptConfigParameters } from '../interface';
import { excludeUnEnabledVariables } from '../utils';
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

interface IProps extends IModalManagerChildrenProps {
  id: string;
}

const ChatConfigurationModal = ({ visible, hideModal, id }: IProps) => {
  const [form] = Form.useForm();
  const [value, setValue] = useState<ConfigurationSegmented>(
    ConfigurationSegmented.AssistantSetting,
  );
  const promptEngineRef = useRef<Array<IPromptConfigParameters>>([]);
  const loading = useOneNamespaceEffectsLoading('chatModel', ['setDialog']);

  const setDialog = useSetDialog();
  const currentDialog = useFetchDialog(id, visible);
  const { resetCurrentDialog } = useResetCurrentDialog();

  const handleOk = async () => {
    const values = await form.validateFields();
    const nextValues: any = omit(values, [
      ...Object.keys(variableEnabledFieldMap),
      'parameters',
      ...excludeUnEnabledVariables(values),
    ]);
    const emptyResponse = nextValues.prompt_config?.empty_response ?? '';

    const fileList = values.icon;
    let icon;

    if (Array.isArray(fileList) && fileList.length > 0) {
      icon = fileList[0].thumbUrl;
    }

    const finalValues = {
      dialog_id: id,
      ...nextValues,
      prompt_config: {
        ...nextValues.prompt_config,
        parameters: promptEngineRef.current,
        empty_response: emptyResponse,
      },
      icon,
    };
    console.info(promptEngineRef.current);
    console.info(nextValues);
    console.info(finalValues);
    const retcode: number = await setDialog(finalValues);
    if (retcode === 0) {
      hideModal();
    }
  };

  const handleCancel = () => {
    hideModal();
  };

  const handleSegmentedChange = (val: SegmentedValue) => {
    setValue(val as ConfigurationSegmented);
  };

  const handleModalAfterClose = () => {
    resetCurrentDialog();
    form.resetFields();
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

  useEffect(() => {
    const icon = currentDialog.icon;
    let fileList: UploadFile[] = [];

    if (icon) {
      fileList = [{ uid: '1', name: 'file', thumbUrl: icon, status: 'done' }];
    }
    form.setFieldsValue({ ...currentDialog, icon: fileList });
  }, [currentDialog, form]);

  return (
    <Modal
      title={title}
      width={688}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={loading}
      destroyOnClose
      afterClose={handleModalAfterClose}
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
