import { KnowledgeRouteKey } from '@/constants/knowledge';
import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import { Button, Form, Input, Radio, Select, Space, Tag } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useDispatch, useNavigate, useSelector } from 'umi';
import Configuration from './configuration';

import styles from './index.less';

const { CheckableTag } = Tag;
const layout = {
  labelCol: { span: 8 },
  wrapperCol: { span: 16 },
  labelAlign: 'left' as const,
};
const { Option } = Select;

const KnowledgeSetting = () => {
  const dispatch = useDispatch();
  const settingModel = useSelector((state: any) => state.settingModel);
  let navigate = useNavigate();
  const { tenantIfo = {} } = settingModel;
  const parser_ids = tenantIfo?.parser_ids ?? '';
  const embd_id = tenantIfo?.embd_id ?? '';
  const [form] = Form.useForm();
  const [selectedTag, setSelectedTag] = useState('');
  const values = Form.useWatch([], form);
  const knowledgeBaseId = useKnowledgeBaseId();

  const getTenantInfo = useCallback(async () => {
    dispatch({
      type: 'settingModel/getTenantInfo',
      payload: {},
    });
    if (knowledgeBaseId) {
      const data = await dispatch<any>({
        type: 'kSModel/getKbDetail',
        payload: {
          kb_id: knowledgeBaseId,
        },
      });
      if (data.retcode === 0) {
        const { description, name, permission, embd_id } = data.data;
        form.setFieldsValue({ description, name, permission, embd_id });
        setSelectedTag(data.data.parser_id);
      }
    }
  }, [knowledgeBaseId, dispatch, form]);

  const onFinish = async () => {
    try {
      await form.validateFields();

      if (knowledgeBaseId) {
        dispatch({
          type: 'kSModel/updateKb',
          payload: {
            ...values,
            parser_id: selectedTag,
            kb_id: knowledgeBaseId,
            embd_id: undefined,
          },
        });
      } else {
        const retcode = await dispatch<any>({
          type: 'kSModel/createKb',
          payload: {
            ...values,
            parser_id: selectedTag,
          },
        });
        if (retcode === 0) {
          navigate(
            `/knowledge/${KnowledgeRouteKey.Dataset}?id=${knowledgeBaseId}`,
          );
        }
      }
    } catch (error) {
      console.warn(error);
    }
  };

  useEffect(() => {
    getTenantInfo();
  }, [getTenantInfo]);

  const handleChange = (tag: string, checked: boolean) => {
    const nextSelectedTag = checked ? tag : selectedTag;
    console.log('You are interested in: ', nextSelectedTag);
    setSelectedTag(nextSelectedTag);
  };

  return (
    <Form
      {...layout}
      form={form}
      name="validateOnly"
      style={{ maxWidth: 1000, padding: 14 }}
    >
      <Form.Item name="name" label="知识库名称" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="description" label="知识库描述">
        <Input.TextArea />
      </Form.Item>
      <Form.Item name="permission" label="可见权限">
        <Radio.Group>
          <Radio value="me">只有我</Radio>
          <Radio value="team">所有团队成员</Radio>
        </Radio.Group>
      </Form.Item>
      <Form.Item
        name="embd_id"
        label="Embedding 模型"
        hasFeedback
        rules={[{ required: true, message: 'Please select your country!' }]}
      >
        <Select placeholder="Please select a country">
          {embd_id.split(',').map((item: string) => {
            return (
              <Option value={item} key={item}>
                {item}
              </Option>
            );
          })}
        </Select>
      </Form.Item>
      <div style={{ marginTop: '5px' }}>
        修改Embedding 模型，请去<span style={{ color: '#1677ff' }}>设置</span>
      </div>
      <Space size={[0, 8]} wrap>
        <div className={styles.tags}>
          {parser_ids.split(',').map((tag: string) => {
            return (
              <CheckableTag
                key={tag}
                checked={selectedTag === tag}
                onChange={(checked) => handleChange(tag, checked)}
              >
                {tag}
              </CheckableTag>
            );
          })}
        </div>
      </Space>
      <Space size={[0, 8]} wrap></Space>
      <div className={styles.preset}>
        <div className={styles.left}>xxxxx文章</div>
        <div className={styles.right}>预估份数</div>
      </div>
      <Form.Item wrapperCol={{ ...layout.wrapperCol, offset: 8 }}>
        <Button type="primary" onClick={onFinish}>
          保存并处理
        </Button>
        <Button htmlType="button" style={{ marginLeft: '20px' }}>
          取消
        </Button>
      </Form.Item>
    </Form>
  );
};

// export default KnowledgeSetting;

export default Configuration;
