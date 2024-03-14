import { UploadFile } from 'antd';

export const transformFile2Base64 = (val: any): Promise<any> => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.readAsDataURL(val);
    reader.onload = (): void => {
      resolve(reader.result);
    };
    reader.onerror = reject;
  });
};

export const transformBase64ToFile = (
  dataUrl: string,
  filename: string = 'file',
) => {
  let arr = dataUrl.split(','),
    bstr = atob(arr[1]),
    n = bstr.length,
    u8arr = new Uint8Array(n);

  const mime = arr[0].match(/:(.*?);/);
  const mimeType = mime ? mime[1] : 'image/png';

  while (n--) {
    u8arr[n] = bstr.charCodeAt(n);
  }
  return new File([u8arr], filename, { type: mimeType });
};

export const normFile = (e: any) => {
  if (Array.isArray(e)) {
    return e;
  }
  return e?.fileList;
};

export const getUploadFileListFromBase64 = (avatar: string) => {
  let fileList: UploadFile[] = [];

  if (avatar) {
    fileList = [{ uid: '1', name: 'file', thumbUrl: avatar, status: 'done' }];
  }

  return fileList;
};

export const getBase64FromUploadFileList = async (fileList?: UploadFile[]) => {
  if (Array.isArray(fileList) && fileList.length > 0) {
    const base64 = await transformFile2Base64(fileList[0].originFileObj);
    return base64;
    // return fileList[0].thumbUrl; TODO: Even JPG files will be converted to base64 parameters in png format
  }

  return '';
};
