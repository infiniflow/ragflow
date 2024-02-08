import { ReactComponent as NavigationPointerIcon } from '@/assets/svg/navigation-pointer.svg';
import { Table, TableProps } from 'antd';

interface DataType {
  key: string;
  name: string;
  hits: number;
  address: string;
  tags: string[];
}

const SelectFiles = () => {
  const columns: TableProps<DataType>['columns'] = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render: (text) => <p>{text}</p>,
    },

    {
      title: 'Hits',
      dataIndex: 'hits',
      key: 'hits',
      width: 80,
    },
    {
      title: 'View',
      key: 'view',
      width: 50,
      render: () => <NavigationPointerIcon />,
    },
  ];

  const rowSelection = {
    onChange: (selectedRowKeys: React.Key[], selectedRows: DataType[]) => {
      console.log(
        `selectedRowKeys: ${selectedRowKeys}`,
        'selectedRows: ',
        selectedRows,
      );
    },
    getCheckboxProps: (record: DataType) => ({
      disabled: record.name === 'Disabled User', // Column configuration not to be checked
      name: record.name,
    }),
  };

  const data: DataType[] = [
    {
      key: '1',
      name: 'John Brown',
      hits: 32,
      address: 'New York No. 1 Lake Park',
      tags: ['nice', 'developer'],
    },
    {
      key: '2',
      name: 'Jim Green',
      hits: 42,
      address: 'London No. 1 Lake Park',
      tags: ['loser'],
    },
    {
      key: '3',
      name: 'Joe Black',
      hits: 32,
      address: 'Sydney No. 1 Lake Park',
      tags: ['cool', 'teacher'],
    },
  ];
  return (
    <Table
      columns={columns}
      dataSource={data}
      showHeader={false}
      rowSelection={rowSelection}
    />
  );
};

export default SelectFiles;
