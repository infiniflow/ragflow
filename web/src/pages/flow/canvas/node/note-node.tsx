import { Flex, Form, Input, Space } from 'antd';
import { NodeProps } from 'reactflow';
import { NodeData } from '../../interface';
import NodeDropdown from './dropdown';

import SvgIcon from '@/components/svg-icon';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useHandleFormValuesChange } from '../../hooks';
import styles from './index.less';

const { TextArea } = Input;

function NoteNode({ data, id }: NodeProps<NodeData>) {
  const { t } = useTranslation();
  const [form] = Form.useForm();

  const { handleValuesChange } = useHandleFormValuesChange(id);

  useEffect(() => {
    form.setFieldsValue(data?.form);
  }, [form, data?.form]);

  return (
    <section className={styles.noteNode}>
      <Flex justify={'space-between'}>
        <Space size={'small'}>
          <SvgIcon name="note" width={14}></SvgIcon>
          <span className={styles.noteTitle}>{t('flow.note')}</span>
        </Space>
        <NodeDropdown id={id}></NodeDropdown>
      </Flex>
      <Form
        onValuesChange={handleValuesChange}
        form={form}
        className={styles.noteForm}
      >
        <Form.Item name="text" noStyle>
          <TextArea rows={3} placeholder={t('flow.notePlaceholder')} />
        </Form.Item>
      </Form>
    </section>
  );
}

export default NoteNode;
