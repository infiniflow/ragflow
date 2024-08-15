import { Authorization } from '@/constants/authorization';
import { useTranslate } from '@/hooks/common-hooks';
import { useRemoveNextDocument } from '@/hooks/document-hooks';
import { getAuthorization } from '@/utils/authorization-util';
import { getExtension } from '@/utils/document-util';
import {
  CloseCircleOutlined,
  LoadingOutlined,
  PlusOutlined,
  UploadOutlined,
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
import get from 'lodash/get';
import { ChangeEventHandler, useCallback, useState } from 'react';
import FileIcon from '../file-icon';

import styles from './index.less';

type FileType = Parameters<GetProp<UploadProps, 'beforeUpload'>>[0];
const { Text } = Typography;

const getFileId = (file: UploadFile) => get(file, 'response.data.0');

interface IProps {
  disabled: boolean;
  value: string;
  sendDisabled: boolean;
  sendLoading: boolean;
  onPressEnter(documentIds: string[]): void;
  onInputChange: ChangeEventHandler<HTMLInputElement>;
  conversationId: string;
}

const getBase64 = (file: FileType): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.readAsDataURL(file as any);
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = (error) => reject(error);
  });

const MessageInput = ({
  disabled,
  value,
  onPressEnter,
  sendDisabled,
  sendLoading,
  onInputChange,
  conversationId,
}: IProps) => {
  const { t } = useTranslate('chat');
  const { removeDocument } = useRemoveNextDocument();

  const [fileList, setFileList] = useState<UploadFile[]>([]);

  const handlePreview = async (file: UploadFile) => {
    if (!file.url && !file.preview) {
      file.preview = await getBase64(file.originFileObj as FileType);
    }
  };

  const handleChange: UploadProps['onChange'] = ({ fileList: newFileList }) => {
    setFileList(newFileList);
  };
  const isUploadingFile = fileList.some((x) => x.status === 'uploading');

  const handlePressEnter = useCallback(async () => {
    if (isUploadingFile) return;
    const ids = fileList.reduce((pre, cur) => {
      return pre.concat(get(cur, 'response.data', []));
    }, []);

    onPressEnter(ids);
    setFileList([]);
  }, [fileList, onPressEnter, isUploadingFile]);

  const handleRemove = useCallback(
    async (file: UploadFile) => {
      const ids = get(file, 'response.data', []);
      if (ids.length) {
        await removeDocument(ids[0]);
        setFileList((preList) => {
          return preList.filter((x) => getFileId(x) !== ids[0]);
        });
      }
    },
    [removeDocument],
  );

  const uploadButton = (
    <button style={{ border: 0, background: 'none' }} type="button">
      <PlusOutlined />
      <div style={{ marginTop: 8 }}>Upload</div>
    </button>
  );

  return (
    <Flex gap={20} vertical className={styles.messageInputWrapper}>
      <Input
        size="large"
        placeholder={t('sendPlaceholder')}
        value={value}
        disabled={disabled}
        suffix={
          <Space>
            <Upload
              action="/v1/document/upload_and_parse"
              // listType="picture-card"
              fileList={fileList}
              onPreview={handlePreview}
              onChange={handleChange}
              multiple
              headers={{ [Authorization]: getAuthorization() }}
              data={{ conversation_id: conversationId }}
              method="post"
              onRemove={handleRemove}
              showUploadList={false}
            >
              <Button icon={<UploadOutlined />}></Button>
            </Upload>
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
      {/* <Upload
        action="/v1/document/upload_and_parse"
        listType="picture-card"
        fileList={fileList}
        onPreview={handlePreview}
        onChange={handleChange}
        multiple
        headers={{ [Authorization]: getAuthorization() }}
        data={{ conversation_id: conversationId }}
        method="post"
        onRemove={handleRemove}
      >
        {fileList.length >= 8 ? null : uploadButton}
      </Upload> */}
      {fileList.length > 0 && (
        <List
          grid={{
            gutter: 16,
            xs: 1,
            sm: 2,
            md: 2,
            lg: 1,
            xl: 2,
            xxl: 4,
          }}
          dataSource={fileList}
          renderItem={(item) => {
            const fileExtension = getExtension(item.name);

            return (
              <List.Item>
                <Card className={styles.documentCard}>
                  <>
                    <Flex gap={10} align="center">
                      {item.status === 'uploading' || !item.response ? (
                        <Spin
                          indicator={
                            <LoadingOutlined style={{ fontSize: 24 }} spin />
                          }
                        />
                      ) : (
                        <FileIcon
                          id={getFileId(item)}
                          name={item.name}
                        ></FileIcon>
                      )}
                      <Flex vertical style={{ width: '90%' }}>
                        <Text
                          ellipsis={{ tooltip: item.name }}
                          className={styles.nameText}
                        >
                          <b> {item.name}</b>
                        </Text>
                        {item.percent !== 100 ? (
                          '上传中'
                        ) : !item.response ? (
                          '解析中'
                        ) : (
                          <Space>
                            <span>{fileExtension?.toUpperCase()},</span>
                          </Space>
                        )}
                      </Flex>
                    </Flex>
                  </>

                  {item.status !== 'uploading' && (
                    <CloseCircleOutlined
                      className={styles.deleteIcon}
                      onClick={() => handleRemove(item)}
                    />
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

export default MessageInput;
