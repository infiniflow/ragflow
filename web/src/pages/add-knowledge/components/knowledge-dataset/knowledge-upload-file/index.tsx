import { ReactComponent as SelectFilesEndIcon } from '@/assets/svg/select-files-end.svg';
import { ReactComponent as SelectFilesStartIcon } from '@/assets/svg/select-files-start.svg';
import {
  useDeleteDocumentById,
  useGetDocumentDefaultParser,
  useKnowledgeBaseId,
} from '@/hooks/knowledgeHook';
import {
  useFetchTenantInfo,
  useSelectParserList,
} from '@/hooks/userSettingHook';

import uploadService from '@/services/uploadService';
import {
  ArrowLeftOutlined,
  DeleteOutlined,
  EditOutlined,
  FileDoneOutlined,
  InboxOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Flex,
  Popover,
  Progress,
  Radio,
  RadioChangeEvent,
  Space,
  Upload,
  UploadFile,
  UploadProps,
} from 'antd';
import classNames from 'classnames';
import { ReactElement, useEffect, useRef, useState } from 'react';
import { Link, useDispatch, useNavigate } from 'umi';

import { KnowledgeRouteKey } from '@/constants/knowledge';
import styles from './index.less';

const { Dragger } = Upload;

type UploadRequestOption = Parameters<
  NonNullable<UploadProps['customRequest']>
>[0];

const UploaderItem = ({
  file,
  actions,
  isUpload,
}: {
  isUpload: boolean;
  originNode: ReactElement;
  file: UploadFile;
  fileList: object[];
  actions: { download: Function; preview: Function; remove: any };
}) => {
  const { parserConfig, defaultParserId } = useGetDocumentDefaultParser(
    file?.response?.kb_id,
  );
  const { removeDocument } = useDeleteDocumentById();
  const [value, setValue] = useState(defaultParserId);
  const dispatch = useDispatch();

  const documentId = file?.response?.id;

  const parserList = useSelectParserList();

  const saveParser = (parserId: string) => {
    dispatch({
      type: 'kFModel/document_change_parser',
      payload: {
        parser_id: parserId,
        doc_id: documentId,
        parser_config: parserConfig,
      },
    });
  };

  const onChange = (e: RadioChangeEvent) => {
    const val = e.target.value;
    setValue(val);
    saveParser(val);
  };

  const content = (
    <Radio.Group onChange={onChange} value={value}>
      <Space direction="vertical">
        {parserList.map(
          (
            x, // value is lowercase, key is uppercase
          ) => (
            <Radio value={x.value} key={x.value}>
              {x.label}
            </Radio>
          ),
        )}
      </Space>
    </Radio.Group>
  );

  const handleRemove = async () => {
    const ret: any = await removeDocument(documentId);
    if (ret === 0) {
      actions?.remove();
    }
  };

  useEffect(() => {
    setValue(defaultParserId);
  }, [defaultParserId]);

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
          <Popover content={content} placement="bottom">
            <EditOutlined />
          </Popover>
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
  const dispatch = useDispatch();

  const navigate = useNavigate();
  const fileListRef = useRef<UploadFile[]>([]);

  const createRequest: (props: UploadRequestOption) => void = async function ({
    file,
    onSuccess,
    onError,
    onProgress,
  }) {
    const { data } = await uploadService.uploadFile(file, knowledgeBaseId);
    if (data.retcode === 0) {
      if (onSuccess) {
        onSuccess(data.data);
      }
    } else {
      if (onError) {
        onError(data.data);
      }
    }
  };

  const props: UploadProps = {
    name: 'file',
    multiple: true,
    itemRender(originNode, file, fileList, actions) {
      fileListRef.current = fileList;
      return (
        <UploaderItem
          isUpload={isUpload}
          file={file}
          fileList={fileList}
          originNode={originNode}
          actions={actions}
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
    dispatch({
      type: 'kFModel/document_run',
      payload: { doc_ids: ids, run: 1 },
    });
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
      <section className={styles.uploadContent}>
        <Dragger
          {...props}
          className={classNames(styles.uploader, {
            [styles.hiddenUploader]: !isUpload,
          })}
        >
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
      <section className={styles.footer}>
        <Button
          type="primary"
          className={styles.nextButton}
          onClick={handleNextClick}
        >
          Next
        </Button>
      </section>
    </div>
  );
};

export default KnowledgeUploadFile;
