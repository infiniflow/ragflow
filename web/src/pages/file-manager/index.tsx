import { Table } from 'antd';
import ActionCell from './action-cell';
import FileToolbar from './file-toolbar';

import { useSelectFileList } from '@/hooks/fileManagerHooks';
import styles from './index.less';

const columns = [
  {
    title: 'Name',
    dataIndex: 'name',
    key: 'name',
  },
  {
    title: 'Upload Date',
    dataIndex: 'create_date',
    key: 'create_date',
  },
  {
    title: 'Location',
    dataIndex: 'location',
    key: 'location',
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
  const fileList = useSelectFileList();

  return (
    <section className={styles.fileManagerWrapper}>
      <FileToolbar selectedRowKeys={[]}></FileToolbar>
      <Table dataSource={fileList} columns={columns} rowKey={'id'} />
    </section>
  );
};

export default FileManager;
