import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { InboxOutlined } from '@ant-design/icons';
import { Modal, Upload, UploadFile, UploadProps } from 'antd';
import { Dispatch, SetStateAction, useState } from 'react';

import { FileMimeType } from '@/constants/common';

import styles from './index.less';

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
  const { t } = useTranslate('fileManager');
  const props: UploadProps = {
    multiple: false,
    accept: FileMimeType.Json,
    onRemove: (file) => {
      const index = fileList.indexOf(file);
      const newFileList = fileList.slice();
      newFileList.splice(index, 1);
      setFileList(newFileList);
    },
    beforeUpload: (file) => {
      setFileList(() => {
        return [file];
      });

      return false;
    },
    directory,
    fileList,
  };

  return (
    <Dragger {...props} className={styles.uploader}>
      <p className="ant-upload-drag-icon">
        <InboxOutlined />
      </p>
      <p className="ant-upload-text">{t('uploadTitle')}</p>
      <p className="ant-upload-hint">{t('uploadDescription')}</p>
      {false && <p className={styles.uploadLimit}>{t('uploadLimit')}</p>}
    </Dragger>
  );
};

const JsonUploadModal = ({
  visible,
  hideModal,
  loading,
  onOk: onFileUploadOk,
}: IModalProps<UploadFile[]>) => {
  const { t } = useTranslate('fileManager');
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [directoryFileList, setDirectoryFileList] = useState<UploadFile[]>([]);

  const clearFileList = () => {
    setFileList([]);
    setDirectoryFileList([]);
  };

  const onOk = async () => {
    const ret = await onFileUploadOk?.([...fileList, ...directoryFileList]);
    return ret;
  };

  const afterClose = () => {
    clearFileList();
  };

  return (
    <Modal
      title={t('uploadFile')}
      open={visible}
      onOk={onOk}
      onCancel={hideModal}
      confirmLoading={loading}
      afterClose={afterClose}
    >
      <FileUpload
        directory={false}
        fileList={fileList}
        setFileList={setFileList}
      ></FileUpload>
    </Modal>
  );
};

export default JsonUploadModal;
