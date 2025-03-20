import { EditableCell, EditableRow } from '@/components/editable-cell';
import { useTranslate } from '@/hooks/common-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { DeleteOutlined } from '@ant-design/icons';
import { Button, Collapse, Flex, Select, Table, TableProps } from 'antd';
import { trim } from 'lodash';
import { useBuildComponentIdSelectOptions } from '../../hooks/use-get-begin-query';
import { IVariable } from '../../interface';
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

const DynamicVariablesForm = ({ node }: IProps) => {
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

  const columns: TableProps<IVariable>['columns'] = [
    {
      title: t('key'),
      dataIndex: 'key',
      key: 'key',
      onCell: (record: IVariable) => ({
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
            showSearch
            options={options.filter((p) => p.key === 'begin')}
            value={text}
            disabled={trim(record.value) !== ''}
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
    <Collapse
      className={styles.dynamicParameterVariable}
      defaultActiveKey={['1']}
      items={[
        {
          key: '1',
          label: (
            <Flex justify={'space-between'}>
              <span className={styles.title}>{t('parameter')}</span>
              <Button size="small" onClick={handleAdd}>
                {t('add')}
              </Button>
            </Flex>
          ),
          children: (
            <Table
              dataSource={dataSource}
              columns={columns}
              rowKey={'id'}
              components={components}
              rowClassName={() => styles.editableRow}
              scroll={{ x: true }}
              bordered
            />
          ),
        },
      ]}
    />
  );
};

export default DynamicVariablesForm;
