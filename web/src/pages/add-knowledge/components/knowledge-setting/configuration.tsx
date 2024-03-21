import { normFile } from '@/utils/fileUtil';
import { PlusOutlined } from '@ant-design/icons';
import {
  Button,
  Flex,
  Form,
  Input,
  InputNumber,
  Radio,
  Select,
  Slider,
  Space,
  Upload,
} from 'antd';
import {
  useFetchKnowledgeConfigurationOnMount,
  useSubmitKnowledgeConfiguration,
} from './hooks';

import { FormInstance } from 'antd/lib';
import styles from './index.less';

const { Option } = Select;

const ConfigurationForm = ({ form }: { form: FormInstance }) => {
  const { submitKnowledgeConfiguration, submitLoading } =
    useSubmitKnowledgeConfiguration();
  const { parserList, embeddingModelOptions } =
    useFetchKnowledgeConfigurationOnMount(form);

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };

  return (
    <Form
      form={form}
      name="validateOnly"
      layout="vertical"
      autoComplete="off"
      onFinish={submitKnowledgeConfiguration}
      onFinishFailed={onFinishFailed}
    >
      <Form.Item
        name="name"
        label="Knowledge base name"
        rules={[{ required: true }]}
      >
        <Input />
      </Form.Item>
      <Form.Item
        name="avatar"
        label="Knowledge base photo"
        valuePropName="fileList"
        getValueFromEvent={normFile}
      >
        <Upload
          listType="picture-card"
          maxCount={1}
          beforeUpload={() => false}
          showUploadList={{ showPreviewIcon: false, showRemoveIcon: false }}
        >
          <button style={{ border: 0, background: 'none' }} type="button">
            <PlusOutlined />
            <div style={{ marginTop: 8 }}>Upload</div>
          </button>
        </Upload>
      </Form.Item>
      <Form.Item name="description" label="Description">
        <Input />
      </Form.Item>
      <Form.Item
        label="Language"
        name="language"
        initialValue={'Chinese'}
        rules={[{ required: true, message: 'Please input your language!' }]}
      >
        <Select placeholder="select your language">
          <Option value="English">English</Option>
          <Option value="Chinese">Chinese</Option>
        </Select>
      </Form.Item>
      <Form.Item
        name="permission"
        label="Permissions"
        tooltip="coming soon"
        rules={[{ required: true }]}
      >
        <Radio.Group>
          <Radio value="me">Only me</Radio>
          <Radio value="team">Team</Radio>
        </Radio.Group>
      </Form.Item>
      <Form.Item
        name="embd_id"
        label="Embedding Model"
        rules={[{ required: true }]}
        tooltip="xx"
      >
        <Select
          placeholder="Please select a country"
          options={embeddingModelOptions}
        ></Select>
      </Form.Item>
      <Form.Item
        name="parser_id"
        label="Chunk method"
        tooltip="xx"
        rules={[{ required: true }]}
      >
        <Select placeholder="Please select a country">
          {parserList.map((x) => (
            <Option value={x.value} key={x.value}>
              {x.label}
            </Option>
          ))}
        </Select>
      </Form.Item>
      <Form.Item noStyle dependencies={['parser_id']}>
        {({ getFieldValue }) => {
          const parserId = getFieldValue('parser_id');

          if (parserId === 'naive') {
            return (
              <Form.Item label="Max token number" tooltip="xxx">
                <Flex gap={20} align="center">
                  <Flex flex={1}>
                    <Form.Item
                      name={['parser_config', 'chunk_token_num']}
                      noStyle
                      initialValue={128}
                      rules={[
                        { required: true, message: 'Province is required' },
                      ]}
                    >
                      <Slider className={styles.variableSlider} max={2048} />
                    </Form.Item>
                  </Flex>
                  <Form.Item
                    name={['parser_config', 'chunk_token_num']}
                    noStyle
                    rules={[{ required: true, message: 'Street is required' }]}
                  >
                    <InputNumber
                      className={styles.sliderInputNumber}
                      max={2048}
                      min={0}
                    />
                  </Form.Item>
                </Flex>
              </Form.Item>
            );
          }
          return null;
        }}
      </Form.Item>
      <Form.Item>
        <div className={styles.buttonWrapper}>
          <Space>
            <Button htmlType="reset" size={'middle'}>
              Cancel
            </Button>
            <Button
              htmlType="submit"
              type="primary"
              size={'middle'}
              loading={submitLoading}
            >
              Save
            </Button>
          </Space>
        </div>
      </Form.Item>
    </Form>
  );
};

export default ConfigurationForm;
