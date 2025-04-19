import { EditableCell, EditableRow } from '@/components/editable-cell';
import { useTranslate } from '@/hooks/common-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { DeleteOutlined } from '@ant-design/icons';
import { Button, Collapse, Flex, Input, Table, TableProps } from 'antd';
import { IInvokeVariable } from '../../interface';
import { useHandleOperateParameters } from './hooks';

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
  const { t } = useTranslate('mcpserver');

  const { dataSource, handleAdd, handleRemove, handleValueChange } =
    useHandleOperateParameters(nodeId!);

  const columns: TableProps<IInvokeVariable>['columns'] = [
    {
      title: t('mcpAddr'),
      dataIndex: 'value',
      key: 'value',
      align: 'center',
      width: 140,
      render(text, record) {
        return (
          <Input
            value={text}
            onChange={handleValueChange(record)}
            placeholder={t('placeholder')}
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
      defaultActiveKey={['1']}
      items={[
        {
          key: '1',
          label: (
            <Flex justify={'space-between'}>
              <span>{t('parameter')}</span>
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
