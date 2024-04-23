import { useSelectFileList } from '@/hooks/fileManagerHooks';
import { IFile } from '@/interfaces/database/file-manager';
import { formatDate } from '@/utils/date';
import { Button, Table } from 'antd';
import { ColumnsType } from 'antd/es/table';
import ActionCell from './action-cell';
import FileToolbar from './file-toolbar';
import {
  useGetRowSelection,
  useNavigateToOtherFolder,
  useRenameCurrentFile,
} from './hooks';

import RenameModal from '@/components/rename-modal';
import styles from './index.less';

const FileManager = () => {
  const fileList = useSelectFileList();
  const rowSelection = useGetRowSelection();
  const navigateToOtherFolder = useNavigateToOtherFolder();
  const {
    fileRenameVisible,
    fileRenameLoading,
    hideFileRenameModal,
    showFileRenameModal,
    initialFileName,
    onFileRenameOk,
  } = useRenameCurrentFile();

  const columns: ColumnsType<IFile> = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render(value, record) {
        return record.type === 'folder' ? (
          <Button
            type={'link'}
            onClick={() => navigateToOtherFolder(record.id)}
          >
            {value}
          </Button>
        ) : (
          value
        );
      },
    },
    {
      title: 'Upload Date',
      dataIndex: 'create_date',
      key: 'create_date',
      render(text) {
        return formatDate(text);
      },
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
      render: (text, record) => (
        <ActionCell
          record={record}
          setCurrentRecord={(record: any) => {
            console.info(record);
          }}
          showRenameModal={showFileRenameModal}
        ></ActionCell>
      ),
    },
  ];

  return (
    <section className={styles.fileManagerWrapper}>
      <FileToolbar
        selectedRowKeys={rowSelection.selectedRowKeys as string[]}
      ></FileToolbar>
      <Table
        dataSource={fileList}
        columns={columns}
        rowKey={'id'}
        rowSelection={rowSelection}
      />
      <RenameModal
        visible={fileRenameVisible}
        hideModal={hideFileRenameModal}
        onOk={onFileRenameOk}
        initialName={initialFileName}
        loading={fileRenameLoading}
      ></RenameModal>
    </section>
  );
};

export default FileManager;
