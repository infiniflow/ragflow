import { Authorization } from '@/constants/authorization';
import { useTranslate } from '@/hooks/common-hooks';
import { useRemoveNextDocument } from '@/hooks/document-hooks';
import { getAuthorization } from '@/utils/authorization-util';
import { PlusOutlined } from '@ant-design/icons';
import type { GetProp, UploadFile } from 'antd';
import { Button, Flex, Input, Upload, UploadProps } from 'antd';
import get from 'lodash/get';
import { ChangeEventHandler, useCallback, useState } from 'react';

type FileType = Parameters<GetProp<UploadProps, 'beforeUpload'>>[0];

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
    console.log('🚀 ~ newFileList:', newFileList);
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
    (file: UploadFile) => {
      const ids = get(file, 'response.data', []);
      if (ids.length) {
        removeDocument(ids[0]);
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
    <Flex gap={10} vertical>
      <Input
        size="large"
        placeholder={t('sendPlaceholder')}
        value={value}
        disabled={disabled}
        suffix={
          <Button
            type="primary"
            onClick={handlePressEnter}
            loading={sendLoading}
            disabled={sendDisabled || isUploadingFile}
          >
            {t('send')}
          </Button>
        }
        onPressEnter={handlePressEnter}
        onChange={onInputChange}
      />
      <Upload
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
      </Upload>
    </Flex>
  );
};

export default MessageInput;
