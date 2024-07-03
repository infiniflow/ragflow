import { EditableCell, EditableRow } from '@/components/editable-cell';
import { useTranslate } from '@/hooks/commonHooks';
import { DeleteOutlined } from '@ant-design/icons';
import { Button, Flex, Select, Table, TableProps } from 'antd';
import { useEffect, useState } from 'react';
import { v4 as uuid } from 'uuid';
import { IGenerateParameter } from '../interface';

import { Operator } from '../constant';
import { useBuildFormSelectOptions } from '../form-hooks';
import styles from './index.less';

interface IProps {
  nodeId?: string;
}

const components = {
  body: {
    row: EditableRow,
    cell: EditableCell,
  },
};

const DynamicParameters = ({ nodeId }: IProps) => {
  const [dataSource, setDataSource] = useState<IGenerateParameter[]>([]);
  const { t } = useTranslate('flow');

  const buildCategorizeToOptions = useBuildFormSelectOptions(
    Operator.Generate,
    nodeId,
  );

  const handleRemove = (id?: string) => () => {
    const newData = dataSource.filter((item) => item.id !== id);
    setDataSource(newData);
  };

  const handleAdd = () => {
    setDataSource((state) => [
      ...state,
      {
        id: uuid(),
        key: '',
        component_id: undefined,
      },
    ]);
  };

  const handleSave = (row: IGenerateParameter) => {
    const newData = [...dataSource];
    const index = newData.findIndex((item) => row.id === item.id);
    const item = newData[index];
    newData.splice(index, 1, {
      ...item,
      ...row,
    });
    setDataSource(newData);
  };

  useEffect(() => {}, [dataSource]);

  const handleOptionalChange = (row: IGenerateParameter) => (value: string) => {
    const newData = [...dataSource];
    const index = newData.findIndex((item) => row.id === item.id);
    const item = newData[index];
    newData.splice(index, 1, {
      ...item,
      component_id: value,
    });
    setDataSource(newData);
  };

  const columns: TableProps<IGenerateParameter>['columns'] = [
    {
      title: t('key'),
      dataIndex: 'key',
      key: 'key',
      onCell: (record: IGenerateParameter) => ({
        record,
        editable: true,
        dataIndex: 'key',
        title: 'key',
        handleSave,
      }),
    },
    {
      title: t('componentId'),
      dataIndex: 'component_id',
      key: 'component_id',
      align: 'center',
      render(text, record) {
        return (
          <Select
            style={{ width: '100%' }}
            allowClear
            options={buildCategorizeToOptions([])}
            // onChange={handleSelectChange(
            //   form.getFieldValue(['parameters', field.name, 'key']),
            // )}
          />
        );
      },
    },
    {
      title: t('operation'),
      dataIndex: 'operation',
      width: 20,
      key: 'operation',
      align: 'center',
      render(_, record) {
        return <DeleteOutlined onClick={handleRemove(record.id)} />;
      },
    },
  ];

  return (
    <section>
      <Flex justify="end">
        <Button size="small" onClick={handleAdd}>
          {t('add')}
        </Button>
      </Flex>
      <Table
        dataSource={dataSource}
        columns={columns}
        rowKey={'id'}
        className={styles.variableTable}
        components={components}
        rowClassName={() => styles.editableRow}
      />
    </section>
  );
};

export default DynamicParameters;
