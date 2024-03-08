import {
  useFetchKnowledgeBaseConfiguration,
  useKnowledgeBaseId,
} from '@/hooks/knowledgeHook';
import {
  useFetchTenantInfo,
  useSelectParserList,
} from '@/hooks/userSettingHook';

import {
  Button,
  Divider,
  Form,
  Input,
  Radio,
  Select,
  Space,
  Typography,
  Upload,
  UploadFile,
} from 'antd';
import pick from 'lodash/pick';
import { useEffect } from 'react';
import { useDispatch, useSelector } from 'umi';

import { useFetchLlmList, useSelectLlmOptions } from '@/hooks/llmHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import {
  getBase64FromUploadFileList,
  getUploadFileListFromBase64,
  normFile,
} from '@/utils/fileUtil';
import { PlusOutlined } from '@ant-design/icons';
import { LlmModelType } from '../../constant';
import styles from './index.less';

const { Title } = Typography;
const { Option } = Select;

const Configuration = () => {
  const [form] = Form.useForm();
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();
  const loading = useOneNamespaceEffectsLoading('kSModel', ['updateKb']);

  const knowledgeDetails: IKnowledge = useSelector(
    (state: any) => state.kSModel.knowledgeDetails,
  );

  const parserList = useSelectParserList();

  const embeddingModelOptions = useSelectLlmOptions();

  const onFinish = async (values: any) => {
    const avatar = getBase64FromUploadFileList(values.avatar);
    dispatch({
      type: 'kSModel/updateKb',
      payload: {
        ...values,
        avatar,
        kb_id: knowledgeBaseId,
      },
    });
  };

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };

  useEffect(() => {
    const fileList: UploadFile[] = getUploadFileListFromBase64(
      knowledgeDetails.avatar,
    );

    form.setFieldsValue({
      ...pick(knowledgeDetails, [
        'description',
        'name',
        'permission',
        'embd_id',
        'parser_id',
      ]),
      avatar: fileList,
    });
  }, [form, knowledgeDetails]);

  useFetchTenantInfo();
  useFetchKnowledgeBaseConfiguration();

  useFetchLlmList(LlmModelType.Embedding);

  return (
    <div className={styles.configurationWrapper}>
      <Title level={5}>Configuration</Title>
      <p>Update your knowledge base details especially parsing method here.</p>
      <Divider></Divider>
      <Form
        form={form}
        name="validateOnly"
        layout="vertical"
        autoComplete="off"
        onFinish={onFinish}
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
            showUploadList={{ showPreviewIcon: false, showRemoveIcon: false }}
          >
            <button style={{ border: 0, background: 'none' }} type="button">
              <PlusOutlined />
              <div style={{ marginTop: 8 }}>Upload</div>
            </button>
          </Upload>
        </Form.Item>
        <Form.Item name="description" label="Knowledge base bio">
          <Input />
        </Form.Item>
        <Form.Item
          name="permission"
          label="Permissions"
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
        >
          <Select
            placeholder="Please select a country"
            options={embeddingModelOptions}
          ></Select>
        </Form.Item>
        <Form.Item
          name="parser_id"
          label="Knowledge base category"
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
                loading={loading}
              >
                Save
              </Button>
            </Space>
          </div>
        </Form.Item>
      </Form>
    </div>
  );
};

export default Configuration;
