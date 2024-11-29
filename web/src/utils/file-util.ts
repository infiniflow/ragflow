import { UploadFile } from 'antd';

export const transformFile2Base64 = (val: any): Promise<any> => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.readAsDataURL(val);
    reader.onload = (): void => {
      // Create image object
      const img = new Image();
      img.src = reader.result as string;

      img.onload = () => {
        // Create canvas
        const canvas = document.createElement('canvas');
        const ctx = canvas.getContext('2d');

        // Calculate compressed dimensions, set max width/height to 800px
        let width = img.width;
        let height = img.height;
        const maxSize = 100;

        if (width > height && width > maxSize) {
          height = (height * maxSize) / width;
          width = maxSize;
        } else if (height > maxSize) {
          width = (width * maxSize) / height;
          height = maxSize;
        }

        // Set canvas dimensions
        canvas.width = width;
        canvas.height = height;

        // Draw image
        ctx?.drawImage(img, 0, 0, width, height);

        // Convert to base64, maintain original format and transparency
        const compressedBase64 = canvas.toDataURL('image/png');
        resolve(compressedBase64);
      };

      img.onerror = reject;
    };
    reader.onerror = reject;
  });
};

export const transformBase64ToFile = (
  dataUrl: string,
  filename: string = 'file',
) => {
  console.log('transformBase64ToFile', dataUrl);
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
  console.log('normFile', e);
  if (Array.isArray(e)) {
    return e;
  }
  return e?.fileList;
};

export const getUploadFileListFromBase64 = (avatar: string) => {
  console.log('getUploadFileListFromBase64', avatar);
  let fileList: UploadFile[] = [];

  if (avatar) {
    fileList = [{ uid: '1', name: 'file', thumbUrl: avatar, status: 'done' }];
  }

  return fileList;
};

export const getBase64FromUploadFileList = async (fileList?: UploadFile[]) => {
  console.log('getBase64FromUploadFileList', fileList);
  if (Array.isArray(fileList) && fileList.length > 0) {
    const file = fileList[0];
    const originFileObj = file.originFileObj;
    if (originFileObj) {
      const base64 = await transformFile2Base64(originFileObj);
      return base64;
    } else {
      return file.thumbUrl;
    }
    // return fileList[0].thumbUrl; TODO: Even JPG files will be converted to base64 parameters in png format
  }

  return '';
};

export const downloadFile = ({
  url,
  filename,
  target,
}: {
  url: string;
  filename?: string;
  target?: string;
}) => {
  console.log('downloadFile', url);
  const downloadElement = document.createElement('a');
  downloadElement.style.display = 'none';
  downloadElement.href = url;
  if (target) {
    downloadElement.target = '_blank';
  }
  downloadElement.rel = 'noopener noreferrer';
  if (filename) {
    downloadElement.download = filename;
  }
  document.body.appendChild(downloadElement);
  downloadElement.click();
  document.body.removeChild(downloadElement);
};

const Units = ['bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];

export const formatBytes = (x: string | number) => {
  console.log('formatBytes', x);
  let l = 0,
    n = (typeof x === 'string' ? parseInt(x, 10) : x) || 0;

  while (n >= 1024 && ++l) {
    n = n / 1024;
  }

  return n.toFixed(n < 10 && l > 0 ? 1 : 0) + ' ' + Units[l];
};
