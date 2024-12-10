import { useTranslate } from '@/hooks/common-hooks';
import {
  useDeleteDocument,
  useFetchDocumentInfosByIds,
  useRemoveNextDocument,
  useUploadAndParseDocument,
} from '@/hooks/document-hooks';
import { getExtension } from '@/utils/document-util';
import { formatBytes } from '@/utils/file-util';
import {
  CloseCircleOutlined,
  InfoCircleOutlined,
  LoadingOutlined,
} from '@ant-design/icons';
import type { GetProp, UploadFile } from 'antd';
import {
  Button,
  Card,
  Flex,
  Input,
  List,
  Space,
  Spin,
  Typography,
  Upload,
  UploadProps,
} from 'antd';
import classNames from 'classnames';
import get from 'lodash/get';
import { Paperclip } from 'lucide-react';
import {
  ChangeEventHandler,
  memo,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import FileIcon from '../file-icon';
import styles from './index.less';

type FileType = Parameters<GetProp<UploadProps, 'beforeUpload'>>[0];
const { Text } = Typography;

const getFileId = (file: UploadFile) => get(file, 'response.data.0');

const getFileIds = (fileList: UploadFile[]) => {
  const ids = fileList.reduce((pre, cur) => {
    return pre.concat(get(cur, 'response.data', []));
  }, []);

  return ids;
};

const isUploadSuccess = (file: UploadFile) => {
  const code = get(file, 'response.code');
  return typeof code === 'number' && code === 0;
};

interface IProps {
  disabled: boolean;
  value: string;
  sendDisabled: boolean;
  sendLoading: boolean;
  onPressEnter(documentIds: string[]): void;
  onInputChange: ChangeEventHandler<HTMLInputElement>;
  conversationId: string;
  uploadMethod?: string;
  isShared?: boolean;
  showUploadIcon?: boolean;
  createConversationBeforeUploadDocument?(message: string): Promise<any>;
}

const getBase64 = (file: FileType): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.readAsDataURL(file as any);
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = (error) => reject(error);
  });

const MessageInput = ({
  isShared = false,
  disabled,
  value,
  onPressEnter,
  sendDisabled,
  sendLoading,
  onInputChange,
  conversationId,
  showUploadIcon = true,
  createConversationBeforeUploadDocument,
  uploadMethod = 'upload_and_parse',
}: IProps) => {
  const { t } = useTranslate('chat');
  const { removeDocument } = useRemoveNextDocument();
  const { deleteDocument } = useDeleteDocument();
  const { data: documentInfos, setDocumentIds } = useFetchDocumentInfosByIds();
  const { uploadAndParseDocument } = useUploadAndParseDocument(uploadMethod);
  const conversationIdRef = useRef(conversationId);
  const [fileList, setFileList] = useState<UploadFile[]>([]);

  const handlePreview = async (file: UploadFile) => {
    if (!file.url && !file.preview) {
      file.preview = await getBase64(file.originFileObj as FileType);
    }
  };

  const handleChange: UploadProps['onChange'] = async ({
    // fileList: newFileList,
    file,
  }) => {
    let nextConversationId: string = conversationId;
    if (createConversationBeforeUploadDocument) {
      const creatingRet = await createConversationBeforeUploadDocument(
        file.name,
      );
      if (creatingRet?.code === 0) {
        nextConversationId = creatingRet.data.id;
      }
    }
    setFileList((list) => {
      list.push({
        ...file,
        status: 'uploading',
        originFileObj: file as any,
      });
      return [...list];
    });

    const ret = await uploadAndParseDocument({
      conversationId: nextConversationId,
      fileList: [file],
    });
    setFileList((list) => {
      const nextList = list.filter((x) => x.uid !== file.uid);
      nextList.push({
        ...file,
        originFileObj: file as any,
        response: ret,
        percent: 100,
        status: ret?.code === 0 ? 'done' : 'error',
      });
      return nextList;
    });
  };

  const isUploadingFile = fileList.some((x) => x.status === 'uploading');

  const handlePressEnter = useCallback(async () => {
    if (isUploadingFile) return;
    const ids = getFileIds(fileList.filter((x) => isUploadSuccess(x)));

    onPressEnter(ids);
    setFileList([]);
  }, [fileList, onPressEnter, isUploadingFile]);

  const handleRemove = useCallback(
    async (file: UploadFile) => {
      const ids = get(file, 'response.data', []);
      // Upload Successfully
      if (Array.isArray(ids) && ids.length) {
        if (isShared) {
          await deleteDocument(ids);
        } else {
          await removeDocument(ids[0]);
        }
        setFileList((preList) => {
          return preList.filter((x) => getFileId(x) !== ids[0]);
        });
      } else {
        // Upload failed
        setFileList((preList) => {
          return preList.filter((x) => x.uid !== file.uid);
        });
      }
    },
    [removeDocument, deleteDocument, isShared],
  );

  const getDocumentInfoById = useCallback(
    (id: string) => {
      return documentInfos.find((x) => x.id === id);
    },
    [documentInfos],
  );

  useEffect(() => {
    const ids = getFileIds(fileList);
    setDocumentIds(ids);
  }, [fileList, setDocumentIds]);

  useEffect(() => {
    if (
      conversationIdRef.current &&
      conversationId !== conversationIdRef.current
    ) {
      setFileList([]);
    }
    conversationIdRef.current = conversationId;
  }, [conversationId, setFileList]);

  return (
    <Flex gap={20} vertical className={styles.messageInputWrapper}>
      <Input
        size="large"
        placeholder={t('sendPlaceholder')}
        value={value}
        disabled={disabled}
        className={classNames({ [styles.inputWrapper]: fileList.length === 0 })}
        suffix={
          <Space>
            {showUploadIcon && (
              <Upload
                onPreview={handlePreview}
                onChange={handleChange}
                multiple={false}
                onRemove={handleRemove}
                showUploadList={false}
                beforeUpload={() => {
                  return false;
                }}
              >
                <Button
                  type={'text'}
                  disabled={disabled}
                  icon={<Paperclip></Paperclip>}
                ></Button>
              </Upload>
            )}
            <Button
              type="primary"
              onClick={handlePressEnter}
              loading={sendLoading}
              disabled={sendDisabled || isUploadingFile}
            >
              {t('send')}
            </Button>
          </Space>
        }
        onPressEnter={handlePressEnter}
        onChange={onInputChange}
      />

      {fileList.length > 0 && (
        <List
          grid={{
            gutter: 16,
            xs: 1,
            sm: 1,
            md: 1,
            lg: 1,
            xl: 2,
            xxl: 4,
          }}
          dataSource={fileList}
          className={styles.listWrapper}
          renderItem={(item) => {
            const id = getFileId(item);
            const documentInfo = getDocumentInfoById(id);
            const fileExtension = getExtension(documentInfo?.name ?? '');
            const fileName = item.originFileObj?.name ?? '';

            return (
              <List.Item>
                <Card className={styles.documentCard}>
                  <Flex gap={10} align="center">
                    {item.status === 'uploading' ? (
                      <Spin
                        indicator={
                          <LoadingOutlined style={{ fontSize: 24 }} spin />
                        }
                      />
                    ) : item.status === 'error' ? (
                      <InfoCircleOutlined size={30}></InfoCircleOutlined>
                    ) : (
                      <FileIcon id={id} name={fileName}></FileIcon>
                    )}
                    <Flex vertical style={{ width: '90%' }}>
                      <Text
                        ellipsis={{ tooltip: fileName }}
                        className={styles.nameText}
                      >
                        <b> {fileName}</b>
                      </Text>
                      {item.status === 'error' ? (
                        t('uploadFailed')
                      ) : (
                        <>
                          {item.percent !== 100 ? (
                            t('uploading')
                          ) : !item.response ? (
                            t('parsing')
                          ) : (
                            <Space>
                              <span>{fileExtension?.toUpperCase()},</span>
                              <span>
                                {formatBytes(
                                  getDocumentInfoById(id)?.size ?? 0,
                                )}
                              </span>
                            </Space>
                          )}
                        </>
                      )}
                    </Flex>
                  </Flex>

                  {item.status !== 'uploading' && (
                    <span className={styles.deleteIcon}>
                      <CloseCircleOutlined onClick={() => handleRemove(item)} />
                    </span>
                  )}
                </Card>
              </List.Item>
            );
          }}
        />
      )}
    </Flex>
  );
};

export default memo(MessageInput);
