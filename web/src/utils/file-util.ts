import { FileMimeType } from '@/constants/common';
import { UploadFile } from '@/interfaces/antd-compat';
import fileManagerService from '@/services/file-manager-service';

export const transformFile2Base64 = (
  val: any,
  imgSize?: number,
): Promise<any> => {
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
        const maxSize = imgSize ?? 100;

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
  const arr = dataUrl.split(','),
    bstr = atob(arr[1]);
  let n = bstr.length;
  const u8arr = new Uint8Array(n);

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

async function fetchPreviewBlob(
  id: string,
  resource: 'document' | 'files',
  mimeType?: FileMimeType,
) {
  const response =
    resource === 'files'
      ? await fileManagerService.getFile({}, id)
      : await fileManagerService.getDocumentFile({}, id);
  const blob = new Blob([response.data], {
    type: mimeType || response.data.type,
  });

  return blob;
}

export async function previewHtmlFile(
  id: string,
  resource: 'document' | 'files' = 'document',
) {
  const blob = await fetchPreviewBlob(id, resource, FileMimeType.Html);
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.click();
  URL.revokeObjectURL(url);
}

export const downloadFileFromBlob = (blob: Blob, name?: string) => {
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  if (name) {
    a.download = name;
  }
  a.click();
  window.URL.revokeObjectURL(url);
};

export const downloadDocument = async ({
  id,
  filename,
}: {
  id: string;
  filename?: string;
}) => {
  const blob = await fetchPreviewBlob(id, 'document');
  downloadFileFromBlob(blob, filename);
};

const Units = ['bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];

export const formatBytes = (x: string | number) => {
  let l = 0,
    n = (typeof x === 'string' ? parseInt(x, 10) : x) || 0;

  while (n >= 1024 && ++l) {
    n = n / 1024;
  }

  return n.toFixed(n < 10 && l > 0 ? 1 : 0) + ' ' + Units[l];
};

export const downloadJsonFile = async (
  data: Record<string, any>,
  fileName: string,
) => {
  // Pretty-print with 2-space indent + sort keys at every depth so
  // the downloaded file is human-readable AND byte-stable across
  // re-exports: a user exporting the same canvas twice gets the
  // exact same bytes (modulo round-trip edits), which makes the
  // file easy to diff in version control and easy to hand-edit.
  // Mirrors the `sort_keys=True` we apply to the testdata fixtures
  // fixtures, so an exported dsl imported in v2 mode and re-
  // exported stays identical at the byte level.
  //
  // JSON.stringify already leaves non-ASCII (e.g. the Chinese
  // prompts we store under `Browser.prompts`) un-escaped by
  // default, so no `ensure_ascii` toggle is needed.
  const blob = new Blob([JSON.stringify(sortKeysDeep(data), null, 2)], {
    type: FileMimeType.Json,
  });
  downloadFileFromBlob(blob, fileName);
};

// sortKeysDeep returns a structural copy of `value` with every
// plain-object key sorted alphabetically. Array element order is
// preserved (semantic — nodes/edges are not interchangeable), but
// each element is recursively sorted if it is itself an object.
// Primitives, `null`, and non-plain objects are passed through
// unchanged. Used to make the exported dsl byte-stable: JSON
// property iteration order is implementation-defined in JS, and
// React Flow nodes carry fields in a stable order today but we
// don't want to depend on that.
const sortKeysDeep = (value: any): any => {
  if (Array.isArray(value)) {
    return value.map(sortKeysDeep);
  }
  if (value !== null && typeof value === 'object') {
    // Only sort plain {…} objects — skip Date, RegExp, Map, etc.
    if (Object.getPrototypeOf(value) !== Object.prototype) {
      return value;
    }
    return Object.keys(value)
      .sort()
      .reduce<Record<string, any>>((acc, key) => {
        acc[key] = sortKeysDeep(value[key]);
        return acc;
      }, {});
  }
  return value;
};

export function transformBase64ToFileWithPreview(
  dataUrl: string,
  filename: string = 'file',
) {
  const file = transformBase64ToFile(dataUrl, filename);

  (file as any).preview = dataUrl;

  return file;
}

export const getBase64FromFileList = async (fileList?: File[]) => {
  if (Array.isArray(fileList) && fileList.length > 0) {
    const file = fileList[0];
    if (file) {
      const base64 = await transformFile2Base64(file);
      return base64;
    }
  }

  return '';
};
