import { UploadOutlined } from '@ant-design/icons';
import { Button, Upload } from 'antd';
import React, { useEffect, useState } from 'react';

const File: React.FC = () => {
  const [fileList, setFileList] = useState([
    {
      uid: '0',
      name: 'xxx.png',
      status: 'uploading',
      percent: 10,
    },
  ]);
  const obj = {
    uid: '-1',
    name: 'yyy.png',
    status: 'done',
    url: 'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png',
    thumbUrl:
      'https://zos.alipayobjects.com/rmsportal/jkjgkEfvpUPVyRjUImniVslZfWPnJuuZ.png',
  };
  useEffect(() => {
    const timer = setInterval(() => {
      setFileList((fileList: any) => {
        const percent = fileList[0]?.percent;
        if (percent + 10 >= 100) {
          clearInterval(timer);
          return [obj];
        }
        const list = [{ ...fileList[0], percent: percent + 10 }];
        console.log(list);
        return list;
      });
    }, 300);
  }, []);
  return (
    <>
      <Upload
        action="https://run.mocky.io/v3/435e224c-44fb-4773-9faf-380c5e6a2188"
        listType="picture"
        fileList={[...fileList]}
        multiple
      >
        <Button icon={<UploadOutlined />}>Upload</Button>
      </Upload>
    </>
  );
};

export default File;
