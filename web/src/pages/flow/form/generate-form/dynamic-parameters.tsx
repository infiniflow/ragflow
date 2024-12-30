import { EditableCell, EditableRow } from '@/components/editable-cell';
import { useTranslate } from '@/hooks/common-hooks';
import { DeleteOutlined } from '@ant-design/icons';
import { Button, Flex, Select, Table, TableProps } from 'antd';
import { useBuildComponentIdSelectOptions } from '../../hooks/use-get-begin-query';
import { IGenerateParameter, RAGFlowNodeType } from '../../interface';
import { useHandleOperateParameters } from './hooks';

import styles from './index.less';
interface IProps {
  node?: RAGFlowNodeType;
}

const components = {
  body: {
    row: EditableRow,
    cell: EditableCell,
  },
};

const DynamicParameters = ({ node }: IProps) => {
  const nodeId = node?.id;
  const { t } = useTranslate('flow');

  const options = useBuildComponentIdSelectOptions(nodeId, node?.parentId);
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
      width: '40%',
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
      width: '40%',
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
      fixed: 'right',
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
        scroll={{ x: true }}
        bordered
      />
    </section>
  );
};

export default DynamicParameters;
