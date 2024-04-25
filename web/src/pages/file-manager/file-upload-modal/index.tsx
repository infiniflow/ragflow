import { IModalProps } from '@/interfaces/common';
import { InboxOutlined } from '@ant-design/icons';
import {
  Flex,
  Modal,
  Segmented,
  Tabs,
  TabsProps,
  Upload,
  UploadFile,
  UploadProps,
} from 'antd';
import { Dispatch, SetStateAction, useState } from 'react';

const { Dragger } = Upload;

const FileUpload = ({
  directory,
  fileList,
  setFileList,
}: {
  directory: boolean;
  fileList: UploadFile[];
  setFileList: Dispatch<SetStateAction<UploadFile[]>>;
}) => {
  const props: UploadProps = {
    multiple: true,
    onRemove: (file) => {
      const index = fileList.indexOf(file);
      const newFileList = fileList.slice();
      newFileList.splice(index, 1);
      setFileList(newFileList);
    },
    beforeUpload: (file) => {
      setFileList((pre) => {
        return [...pre, file];
      });

      return false;
    },
    directory,
    fileList,
  };

  return (
    <Dragger {...props}>
      <p className="ant-upload-drag-icon">
        <InboxOutlined />
      </p>
      <p className="ant-upload-text">
        Click or drag file to this area to upload
      </p>
      <p className="ant-upload-hint">
        Support for a single or bulk upload. Strictly prohibited from uploading
        company data or other banned files.
      </p>
    </Dragger>
  );
};

const FileUploadModal = ({
  visible,
  hideModal,
  loading,
  onOk: onFileUploadOk,
}: IModalProps<UploadFile[]>) => {
  const [value, setValue] = useState<string | number>('local');
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [directoryFileList, setDirectoryFileList] = useState<UploadFile[]>([]);

  const onOk = () => {
    return onFileUploadOk?.([...fileList, ...directoryFileList]);
  };

  const items: TabsProps['items'] = [
    {
      key: '1',
      label: 'File',
      children: (
        <FileUpload
          directory={false}
          fileList={fileList}
          setFileList={setFileList}
        ></FileUpload>
      ),
    },
    {
      key: '2',
      label: 'Directory',
      children: (
        <FileUpload
          directory
          fileList={directoryFileList}
          setFileList={setDirectoryFileList}
        ></FileUpload>
      ),
    },
  ];

  return (
    <>
      <Modal
        title="File upload"
        open={visible}
        onOk={onOk}
        onCancel={hideModal}
        confirmLoading={loading}
      >
        <Flex gap={'large'} vertical>
          <Segmented
            options={[
              { label: 'Local uploads', value: 'local' },
              { label: 'S3 uploads', value: 's3' },
            ]}
            block
            value={value}
            onChange={setValue}
          />
          {value === 'local' ? (
            <Tabs defaultActiveKey="1" items={items} />
          ) : (
            'coming soon'
          )}
        </Flex>
      </Modal>
    </>
  );
};

export default FileUploadModal;
