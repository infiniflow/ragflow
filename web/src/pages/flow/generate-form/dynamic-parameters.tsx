import { EditableCell, EditableRow } from '@/components/editable-cell';
import { useTranslate } from '@/hooks/common-hooks';
import { DeleteOutlined } from '@ant-design/icons';
import { Button, Flex, Select, Table, TableProps } from 'antd';
import { IGenerateParameter } from '../interface';

import {
  useBuildComponentIdSelectOptions,
  useHandleOperateParameters,
} from './hooks';
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
  const { t } = useTranslate('flow');

  const options = useBuildComponentIdSelectOptions(nodeId);
  const {
    dataSource,
    handleAdd,
    handleRemove,
    handleSave,
    handleComponentIdChange,
  } = useHandleOperateParameters(nodeId!);

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
            options={options}
            value={text}
            onChange={handleComponentIdChange(record)}
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
