import { EditableCell, EditableRow } from '@/components/editable-cell';
import { useTranslate } from '@/hooks/common-hooks';
import { DeleteOutlined } from '@ant-design/icons';
import { Button, Collapse, Flex, Input, Select, Table, TableProps } from 'antd';
import { useBuildComponentIdSelectOptions } from '../../hooks';
import { IInvokeVariable } from '../../interface';
import { useHandleOperateParameters } from './hooks';

import { trim } from 'lodash';
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

const DynamicVariablesForm = ({ nodeId }: IProps) => {
  const { t } = useTranslate('flow');

  const options = useBuildComponentIdSelectOptions(nodeId);
  const {
    dataSource,
    handleAdd,
    handleRemove,
    handleSave,
    handleComponentIdChange,
    handleValueChange,
  } = useHandleOperateParameters(nodeId!);

  const columns: TableProps<IInvokeVariable>['columns'] = [
    {
      title: t('key'),
      dataIndex: 'key',
      key: 'key',
      onCell: (record: IInvokeVariable) => ({
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
      width: 140,
      render(text, record) {
        return (
          <Select
            style={{ width: '100%' }}
            allowClear
            options={options}
            value={text}
            disabled={trim(record.value) !== ''}
            onChange={handleComponentIdChange(record)}
          />
        );
      },
    },
    {
      title: t('value'),
      dataIndex: 'value',
      key: 'value',
      align: 'center',
      width: 140,
      render(text, record) {
        return (
          <Input
            value={text}
            disabled={!!record.component_id}
            onChange={handleValueChange(record)}
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
