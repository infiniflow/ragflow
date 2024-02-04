import { ReactComponent as SelectFilesEndIcon } from '@/assets/svg/select-files-end.svg';
import { ReactComponent as SelectFilesStartIcon } from '@/assets/svg/select-files-start.svg';
import {
  useDeleteDocumentById,
  useKnowledgeBaseId,
} from '@/hooks/knowledgeHook';
import uploadService from '@/services/uploadService';
import { ArrowLeftOutlined, InboxOutlined } from '@ant-design/icons';
import { Flex, Progress, Space, Upload, UploadProps } from 'antd';
import { Link } from 'umi';

import styles from './index.less';

const { Dragger } = Upload;

type UploadRequestOption = Parameters<
  NonNullable<UploadProps['customRequest']>
>[0];

const KnowledgeUploadFile = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const { removeDocument } = useDeleteDocumentById();

  const createRequest: (props: UploadRequestOption) => void = async function ({
    file,
    onSuccess,
    onError,
    onProgress,
  }) {
    const { data } = await uploadService.uploadFile(file, knowledgeBaseId);
    if (data.retcode === 0) {
      onSuccess && onSuccess(data.data);
    } else {
      onError && onError(data.data);
    }
  };

  const props: UploadProps = {
    name: 'file',
    multiple: true,
    customRequest: createRequest,
    onDrop(e) {
      console.log('Dropped files', e.dataTransfer.files);
    },
    async onRemove(file) {
      console.info(file);
      const ret: any = await removeDocument(file.response.id);
      return ret === 0;
    },
  };

  return (
    <div className={styles.uploadWrapper}>
      <section>
        <Space className={styles.backToList}>
          <ArrowLeftOutlined />
          <Link to={`/knowledge/dataset?id=${knowledgeBaseId}`}>
            Back to select files
          </Link>
        </Space>
        <div className={styles.progressWrapper}>
          <Flex align="center" justify="center">
            <SelectFilesStartIcon></SelectFilesStartIcon>
            <Progress
              percent={100}
              showInfo={false}
              className={styles.progress}
              strokeColor="
               rgba(127, 86, 217, 1)
              "
            />
            <SelectFilesEndIcon></SelectFilesEndIcon>
          </Flex>
          <Flex justify="space-around">
            <p className={styles.selectFilesText}>
              <b>Select files</b>
            </p>
            <p className={styles.changeSpecificCategoryText}>
              <b>Change specific category</b>
            </p>
          </Flex>
        </div>
      </section>
      <section>
        <Dragger {...props}>
          <p className="ant-upload-drag-icon">
            <InboxOutlined />
          </p>
          <p className="ant-upload-text">
            Click or drag file to this area to upload
          </p>
          <p className="ant-upload-hint">
            Support for a single or bulk upload. Strictly prohibited from
            uploading company data or other banned files.
          </p>
        </Dragger>
      </section>
    </div>
  );
};

export default KnowledgeUploadFile;
