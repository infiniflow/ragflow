import { Table } from 'antd';
import ActionCell from './action-cell';
import FileToolbar from './file-toolbar';

import styles from './index.less';

const dataSource = [
  {
    key: '1',
    name: '胡彦斌',
    age: 32,
    address: '西湖区湖底公园1号',
  },
  {
    key: '2',
    name: '胡彦祖',
    age: 42,
    address: '西湖区湖底公园1号',
  },
];

const columns = [
  {
    title: 'Name',
    dataIndex: 'name',
    key: 'name',
  },
  {
    title: 'Upload Date',
    dataIndex: 'age',
    key: 'age',
  },
  {
    title: 'Location',
    dataIndex: 'address',
    key: 'address',
  },
  {
    title: 'Action',
    dataIndex: 'action',
    key: 'action',
    render: () => (
      <ActionCell
        record={{}}
        setCurrentRecord={(record: any) => {
          console.info(record);
        }}
        showRenameModal={() => {}}
      ></ActionCell>
    ),
  },
];

const FileManager = () => {
  return (
    <section className={styles.fileManagerWrapper}>
      <FileToolbar selectedRowKeys={[]}></FileToolbar>
      <Table dataSource={dataSource} columns={columns} />;
    </section>
  );
};

export default FileManager;
