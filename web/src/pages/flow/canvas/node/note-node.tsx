import { Flex, Form, Input } from 'antd';
import classNames from 'classnames';
import { NodeProps, NodeResizeControl } from 'reactflow';
import { NodeData } from '../../interface';
import NodeDropdown from './dropdown';

import SvgIcon from '@/components/svg-icon';
import { memo, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useHandleFormValuesChange,
  useHandleNodeNameChange,
} from '../../hooks';
import styles from './index.less';

const { TextArea } = Input;

const controlStyle = {
  background: 'transparent',
  border: 'none',
};

function NoteNode({ data, id }: NodeProps<NodeData>) {
  const { t } = useTranslation();
  const [form] = Form.useForm();

  const { name, handleNameBlur, handleNameChange } = useHandleNodeNameChange({
    id,
    data,
  });
  const { handleValuesChange } = useHandleFormValuesChange(id);

  useEffect(() => {
    form.setFieldsValue(data?.form);
  }, [form, data?.form]);

  return (
    <>
      <NodeResizeControl style={controlStyle} minWidth={190} minHeight={128}>
        <SvgIcon
          name="resize"
          width={12}
          style={{
            position: 'absolute',
            right: 5,
            bottom: 5,
            cursor: 'nwse-resize',
          }}
        ></SvgIcon>
      </NodeResizeControl>
      <section className={styles.noteNode}>
        <Flex
          justify={'space-between'}
          className={classNames(styles.noteTitle, 'note-drag-handle')}
          align="center"
          gap={6}
        >
          <SvgIcon name="note" width={14}></SvgIcon>
          <Input
            value={name ?? t('flow.note')}
            onBlur={handleNameBlur}
            onChange={handleNameChange}
            className={styles.noteName}
          ></Input>
          <NodeDropdown id={id}></NodeDropdown>
        </Flex>
        <Form
          onValuesChange={handleValuesChange}
          form={form}
          className={styles.noteForm}
        >
          <Form.Item name="text" noStyle>
            <TextArea
              rows={3}
              placeholder={t('flow.notePlaceholder')}
              className={styles.noteTextarea}
            />
          </Form.Item>
        </Form>
      </section>
    </>
  );
}

export default memo(NoteNode);
