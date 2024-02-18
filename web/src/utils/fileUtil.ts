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
