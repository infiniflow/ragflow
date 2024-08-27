import { Authorization } from '@/constants/authorization';
import { getAuthorization } from '@/utils/authorization-util';
import { PlusOutlined } from '@ant-design/icons';
import type { UploadFile, UploadProps } from 'antd';
import { Image, Input, Upload } from 'antd';
import { useState } from 'react';

const InputWithUpload = () => {
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewImage, setPreviewImage] = useState('');
  const [fileList, setFileList] = useState<UploadFile[]>([]);

  const handleChange: UploadProps['onChange'] = ({ fileList: newFileList }) =>
    setFileList(newFileList);

  const uploadButton = (
    <button style={{ border: 0, background: 'none' }} type="button">
      <PlusOutlined />
      <div style={{ marginTop: 8 }}>Upload</div>
    </button>
  );
  return (
    <>
      <Input placeholder="Basic usage"></Input>
      <Upload
        action="/v1/document/upload_and_parse"
        listType="picture-card"
        fileList={fileList}
        onChange={handleChange}
        multiple
        headers={{ [Authorization]: getAuthorization() }}
        data={{ conversation_id: '9e9f7d2453e511efb18efa163e197198' }}
        method="post"
      >
        {fileList.length >= 8 ? null : uploadButton}
      </Upload>
      {previewImage && (
        <Image
          wrapperStyle={{ display: 'none' }}
          preview={{
            visible: previewOpen,
            onVisibleChange: (visible) => setPreviewOpen(visible),
            afterOpenChange: (visible) => !visible && setPreviewImage(''),
          }}
          src={previewImage}
        />
      )}
    </>
  );
};

export default () => {
  return (
    <section style={{ height: 500, width: 400 }}>
      <div style={{ height: 200 }}></div>
      <InputWithUpload></InputWithUpload>
    </section>
  );
};
