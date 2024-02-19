import {
  Button,
  Col,
  Divider,
  Form,
  Input,
  Row,
  Select,
  Switch,
  Table,
  TableProps,
} from 'antd';
import { v4 as uuid } from 'uuid';
import { ISegmentedContentProps } from './interface';

import { DeleteOutlined } from '@ant-design/icons';
import { useState } from 'react';
import { EditableCell, EditableRow } from './editable-cell';
import styles from './index.less';

interface DataType {
  key: string;
  optional: boolean;
}

const PromptEngine = ({ show }: ISegmentedContentProps) => {
  const [dataSource, setDataSource] = useState<DataType[]>([]);

  const components = {
    body: {
      row: EditableRow,
      cell: EditableCell,
    },
  };

  const handleRemove = (key: string) => () => {
    const newData = dataSource.filter((item) => item.key !== key);
    setDataSource(newData);
  };

  const handleSave = (row: DataType) => {
    const newData = [...dataSource];
    const index = newData.findIndex((item) => row.key === item.key);
    const item = newData[index];
    newData.splice(index, 1, {
      ...item,
      ...row,
    });
    setDataSource(newData);
  };

  const columns: TableProps<DataType>['columns'] = [
    {
      title: 'key',
      dataIndex: 'variable',
      key: 'variable',
      onCell: (record: DataType) => ({
        record,
        editable: true,
        dataIndex: 'variable',
        title: 'key',
        handleSave,
      }),
    },
    {
      title: 'optional',
      dataIndex: 'optional',
      key: 'optional',
      width: 40,
      align: 'center',
      render() {
        return <Switch size="small" />;
      },
    },
    {
      title: 'operation',
      dataIndex: 'operation',
      width: 30,
      key: 'operation',
      align: 'center',
      render(_, record) {
        return <DeleteOutlined onClick={handleRemove(record.key)} />;
      },
    },
  ];

  const handleAdd = () => {
    setDataSource((state) => [
      ...state,
      {
        key: uuid(),
        variable: '',
        optional: true,
      },
    ]);
  };

  return (
    <>
      <Form.Item
        label="Orchestrate"
        name="orchestrate"
        hidden={!show}
        rules={[{ required: true, message: 'Please input!' }]}
      >
        <Input.TextArea />
      </Form.Item>
      <Divider></Divider>
      <section className={styles.variableContainer}>
        <Row align={'middle'} justify="end">
          <Col span={6} className={styles.variableAlign}>
            <label className={styles.variableLabel}>Variables</label>
          </Col>
          <Col span={18} className={styles.variableAlign}>
            <Button size="small" onClick={handleAdd}>
              Add
            </Button>
          </Col>
        </Row>
        {dataSource.length > 0 && (
          <Row>
            <Col span={6}></Col>
            <Col span={18}>
              <Table
                dataSource={dataSource}
                columns={columns}
                rowKey={'key'}
                className={styles.variableTable}
                components={components}
                rowClassName={() => styles.editableRow}
              />
            </Col>
          </Row>
        )}
      </section>
      <Form.Item
        label="Select one context"
        name="context"
        hidden={!show}
        rules={[{ required: true, message: 'Please input!' }]}
      >
        <Select />
      </Form.Item>
    </>
  );
};

export default PromptEngine;
