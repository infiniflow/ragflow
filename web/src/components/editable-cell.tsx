import { Form, FormInstance, Input, InputRef, Typography } from 'antd';
import React, { useContext, useEffect, useRef, useState } from 'react';

const EditableContext = React.createContext<FormInstance<any> | null>(null);
const { Paragraph, Text } = Typography;

interface EditableRowProps {
  index: number;
}

interface Item {
  key: string;
  name: string;
  age: string;
  address: string;
}

export const EditableRow: React.FC<EditableRowProps> = ({
  index,
  ...props
}) => {
  const [form] = Form.useForm();
  return (
    <Form form={form} component={false}>
      <EditableContext.Provider value={form}>
        <tr {...props} />
      </EditableContext.Provider>
    </Form>
  );
};

interface EditableCellProps {
  title: React.ReactNode;
  editable: boolean;
  children: React.ReactNode;
  dataIndex: keyof Item;
  record: Item;
  handleSave: (record: Item) => void;
}

export const EditableCell: React.FC<EditableCellProps> = ({
  title,
  editable,
  children,
  dataIndex,
  record,
  handleSave,
  ...restProps
}) => {
  const [editing, setEditing] = useState(false);
  const inputRef = useRef<InputRef>(null);
  const form = useContext(EditableContext)!;

  useEffect(() => {
    if (editing) {
      inputRef.current!.focus();
    }
  }, [editing]);

  const toggleEdit = () => {
    setEditing(!editing);
    form.setFieldsValue({ [dataIndex]: record[dataIndex] });
  };

  const save = async () => {
    try {
      const values = await form.validateFields();

      toggleEdit();
      handleSave({ ...record, ...values });
    } catch (errInfo) {
      console.log('Save failed:', errInfo);
    }
  };

  let childNode = children;

  if (editable) {
    childNode = editing ? (
      <Form.Item
        style={{ margin: 0, minWidth: 70 }}
        name={dataIndex}
        rules={[
          {
            required: true,
            message: `${title} is required.`,
          },
        ]}
      >
        <Input ref={inputRef} onPressEnter={save} onBlur={save} />
      </Form.Item>
    ) : (
      <div
        className="editable-cell-value-wrap"
        // style={{ paddingRight: 24 }}
        onClick={toggleEdit}
      >
        <Text ellipsis={{ tooltip: children }} style={{ width: 70 }}>
          {children}
        </Text>
      </div>
    );
  }

  return <td {...restProps}>{childNode}</td>;
};
