import {
  useFetchParserList,
  useKnowledgeBaseId,
  useSelectParserList,
} from '@/hooks/knowledgeHook';
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
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSelector } from 'umi';

import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { IThirdOAIModelCollection } from '@/interfaces/database/llm';
import { PlusOutlined } from '@ant-design/icons';
import styles from './index.less';

const { Title } = Typography;
const { Option } = Select;

const Configuration = () => {
  const [form] = Form.useForm();
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();
  const loading = useOneNamespaceEffectsLoading('kSModel', ['updateKb']);

  const llmInfo: IThirdOAIModelCollection = useSelector(
    (state: any) => state.settingModel.llmInfo,
  );

  const normFile = (e: any) => {
    if (Array.isArray(e)) {
      return e;
    }
    return e?.fileList;
  };

  const parserList = useSelectParserList();

  const embeddingModelOptions = useMemo(() => {
    return Object.entries(llmInfo).map(([key, value]) => {
      return {
        label: key,
        options: value.map((x) => ({
          label: x.llm_name,
          value: x.llm_name,
        })),
      };
    });
  }, [llmInfo]);

  const onFinish = async (values: any) => {
    console.info(values);
    const fileList = values.avatar;
    let avatar;

    if (Array.isArray(fileList)) {
      avatar = fileList[0].thumbUrl;
    }

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

  useFetchParserList();

  const fetchLlmList = useCallback(async () => {
    const data = await dispatch<any>({
      type: 'kSModel/getKbDetail',
      payload: {
        kb_id: knowledgeBaseId,
      },
    });
    const fileList: UploadFile[] = [
      { uid: '1', name: 'file', thumbUrl: data.data.avatar, status: 'done' },
    ];
    if (data.retcode === 0) {
      form.setFieldsValue({
        ...pick(data.data, [
          'description',
          'name',
          'permission',
          'embd_id',
          'parser_id',
        ]),
        avatar: fileList,
      });
    }
  }, [dispatch, knowledgeBaseId, form]);

  const fetchKnowledgeBaseConfiguration = useCallback(() => {
    dispatch({
      type: 'settingModel/llm_list',
      payload: { model_type: 'embedding' },
    });
  }, [dispatch]);

  useEffect(() => {
    fetchKnowledgeBaseConfiguration();
    fetchLlmList();
  }, [fetchLlmList, fetchKnowledgeBaseConfiguration]);

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
