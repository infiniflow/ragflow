import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import uploadService from '@/services/uploadService';
import type { UploadProps } from 'antd';
import React from 'react';
import { Link } from 'umi';
interface PropsType {
  kb_id: string;
  getKfList: () => void;
}

type UploadRequestOption = Parameters<
  NonNullable<UploadProps['customRequest']>
>[0];

const FileUpload: React.FC<PropsType> = ({ kb_id, getKfList }) => {
  const knowledgeBaseId = useKnowledgeBaseId();

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
    // <Upload {...uploadProps}>
    <Link to={`/knowledge/dataset/upload?id=${knowledgeBaseId}`}>导入文件</Link>
    // </Upload>
  );
};

export default FileUpload;
