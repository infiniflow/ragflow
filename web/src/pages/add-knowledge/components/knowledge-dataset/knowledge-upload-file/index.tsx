import { ReactComponent as SelectFilesEndIcon } from '@/assets/svg/select-files-end.svg';
import { ReactComponent as SelectFilesStartIcon } from '@/assets/svg/select-files-start.svg';
import ChunkMethodModal from '@/components/chunk-method-modal';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import {
  useRunDocument,
  useSelectDocumentList,
  useUploadDocument,
} from '@/hooks/documentHooks';
import {
  useDeleteDocumentById,
  useFetchKnowledgeDetail,
  useKnowledgeBaseId,
} from '@/hooks/knowledgeHook';
import {
  useChangeDocumentParser,
  useSetSelectedRecord,
} from '@/hooks/logicHooks';
import { useFetchTenantInfo } from '@/hooks/userSettingHook';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { getExtension, isFileUploadDone } from '@/utils/documentUtils';
import {
  ArrowLeftOutlined,
  CloudUploadOutlined,
  DeleteOutlined,
  EditOutlined,
  FileDoneOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Flex,
  Progress,
  Space,
  Upload,
  UploadFile,
  UploadProps,
} from 'antd';
import classNames from 'classnames';
import { ReactElement, useCallback, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from 'umi';

import styles from './index.less';

const { Dragger } = Upload;

type UploadRequestOption = Parameters<
  NonNullable<UploadProps['customRequest']>
>[0];

const UploaderItem = ({
  file,
  isUpload,
  remove,
  handleEdit,
}: {
  isUpload: boolean;
  originNode: ReactElement;
  file: UploadFile;
  fileList: object[];
  showModal: () => void;
  remove: (id: string) => void;
  setRecord: (record: IKnowledgeFile) => void;
  handleEdit: (id: string) => void;
}) => {
  const { removeDocument } = useDeleteDocumentById();

  const documentId = file?.response?.id;

  const handleRemove = async () => {
    if (file.status === 'error') {
      remove(documentId);
    } else {
      const ret: any = await removeDocument(documentId);
      if (ret === 0) {
        remove(documentId);
      }
    }
  };

  const handleEditClick = () => {
    if (file.status === 'done') {
      handleEdit(documentId);
    }
  };

  return (
    <Card className={styles.uploaderItem}>
      <Flex justify="space-between">
        <FileDoneOutlined className={styles.fileIcon} />
        <section className={styles.uploaderItemTextWrapper}>
          <div>
            <b>{file.name}</b>
          </div>
          <span>{file.size}</span>
        </section>
        {isUpload ? (
          <DeleteOutlined
            className={styles.deleteIcon}
            onClick={handleRemove}
          />
        ) : (
          <EditOutlined onClick={handleEditClick} />
        )}
      </Flex>
      <Flex>
        <Progress
          showInfo={false}
          percent={100}
          className={styles.uploaderItemProgress}
          strokeColor="
        rgba(127, 86, 217, 1)
        "
        />
        <span>100%</span>
      </Flex>
    </Card>
  );
};

const KnowledgeUploadFile = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [isUpload, setIsUpload] = useState(true);
  const [uploadedFileIds, setUploadedFileIds] = useState<string[]>([]);
  const fileListRef = useRef<UploadFile[]>([]);
  const navigate = useNavigate();
  const { currentRecord, setRecord } = useSetSelectedRecord();
  const {
    changeParserLoading,
    onChangeParserOk,
    changeParserVisible,
    hideChangeParserModal,
    showChangeParserModal,
  } = useChangeDocumentParser(currentRecord.id);
  const documentList = useSelectDocumentList();
  const runDocumentByIds = useRunDocument();
  const uploadDocument = useUploadDocument();

  const enabled = useMemo(() => {
    if (isUpload) {
      return (
        uploadedFileIds.length > 0 &&
        fileListRef.current.filter((x) => isFileUploadDone(x)).length ===
          uploadedFileIds.length
      );
    }
    return true;
  }, [uploadedFileIds, isUpload]);

  const createRequest: (props: UploadRequestOption) => void = async function ({
    file,
    onSuccess,
    onError,
    // onProgress,
  }) {
    const data = await uploadDocument(file as UploadFile);
    if (data?.retcode === 0) {
      setUploadedFileIds((pre) => {
        return pre.concat(data.data.id);
      });
      if (onSuccess) {
        onSuccess(data.data);
      }
    } else {
      if (onError) {
        onError(data?.data);
      }
    }
  };

  const removeIdFromUploadedIds = useCallback((id: string) => {
    setUploadedFileIds((pre) => {
      return pre.filter((x) => x !== id);
    });
  }, []);

  const handleItemEdit = useCallback(
    (id: string) => {
      const document = documentList.find((x) => x.id === id);
      if (document) {
        setRecord(document);
      }
      showChangeParserModal();
    },
    [documentList, showChangeParserModal, setRecord],
  );

  const props: UploadProps = {
    name: 'file',
    multiple: true,
    itemRender(originNode, file, fileList, actions) {
      fileListRef.current = fileList;
      const remove = (id: string) => {
        if (isFileUploadDone(file)) {
          removeIdFromUploadedIds(id);
        }
        actions.remove();
      };
      return (
        <UploaderItem
          isUpload={isUpload}
          file={file}
          fileList={fileList}
          originNode={originNode}
          remove={remove}
          showModal={showChangeParserModal}
          setRecord={setRecord}
          handleEdit={handleItemEdit}
        ></UploaderItem>
      );
    },
    customRequest: createRequest,
    onDrop(e) {
      console.log('Dropped files', e.dataTransfer.files);
    },
  };

  const runSelectedDocument = () => {
    const ids = fileListRef.current.map((x) => x.response.id);
    runDocumentByIds(ids);
  };

  const handleNextClick = () => {
    if (!isUpload) {
      runSelectedDocument();
      navigate(`/knowledge/${KnowledgeRouteKey.Dataset}?id=${knowledgeBaseId}`);
    } else {
      setIsUpload(false);
    }
  };

  useFetchTenantInfo();
  useFetchKnowledgeDetail();

  return (
    <>
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
        <section className={styles.uploadContent}>
          <Dragger
            {...props}
            className={classNames(styles.uploader, {
              [styles.hiddenUploader]: !isUpload,
            })}
          >
            <Button className={styles.uploaderButton}>
              <CloudUploadOutlined className={styles.uploaderIcon} />
            </Button>
            <p className="ant-upload-text">
              Click or drag file to this area to upload
            </p>
            <p className="ant-upload-hint">
              Support for a single or bulk upload. Strictly prohibited from
              uploading company data or other banned files.
            </p>
          </Dragger>
        </section>
        <section className={styles.footer}>
          <Button
            type="primary"
            className={styles.nextButton}
            onClick={handleNextClick}
            disabled={!enabled}
            size="large"
          >
            Next
          </Button>
        </section>
      </div>
      <ChunkMethodModal
        documentId={currentRecord.id}
        parserId={currentRecord.parser_id}
        parserConfig={currentRecord.parser_config}
        documentExtension={getExtension(currentRecord.name)}
        onOk={onChangeParserOk}
        visible={changeParserVisible}
        hideModal={hideChangeParserModal}
        loading={changeParserLoading}
      />
    </>
  );
};

export default KnowledgeUploadFile;
