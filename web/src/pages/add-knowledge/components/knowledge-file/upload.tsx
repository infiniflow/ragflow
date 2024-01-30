import uploadService from '@/services/uploadService';
import type { UploadProps } from 'antd';
import { Button, Upload } from 'antd';
import React from 'react';
interface PropsType {
  kb_id: string;
  getKfList: () => void;
}

type UploadRequestOption = Parameters<
  NonNullable<UploadProps['customRequest']>
>[0];

const FileUpload: React.FC<PropsType> = ({ kb_id, getKfList }) => {
  const createRequest: (props: UploadRequestOption) => void = async function ({
    file,
    onSuccess,
    onError,
  }) {
    const { retcode, data } = await uploadService.uploadFile(file, kb_id);
    if (retcode === 0) {
      onSuccess && onSuccess(data, file);
    } else {
      onError && onError(data);
    }
    getKfList && getKfList();
  };
  const uploadProps: UploadProps = {
    customRequest: createRequest,
    showUploadList: false,
  };
  return (
    <Upload {...uploadProps}>
      <Button type="link">导入文件</Button>
    </Upload>
  );
};

export default FileUpload;
