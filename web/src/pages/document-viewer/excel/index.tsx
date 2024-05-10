import jsPreviewExcel from '@js-preview/excel';
import '@js-preview/excel/lib/index.css';
import { useEffect } from 'react';

const Excel = ({ filePath }: { filePath: string }) => {
  const fetchDocument = async () => {
    const myExcelPreviewer = jsPreviewExcel.init(
      document.getElementById('excel'),
    );
    const jsonFile = new XMLHttpRequest();
    jsonFile.open('GET', filePath, true);
    jsonFile.send();
    jsonFile.responseType = 'arraybuffer';
    jsonFile.onreadystatechange = () => {
      if (jsonFile.readyState === 4 && jsonFile.status === 200) {
        myExcelPreviewer
          .preview(jsonFile.response)
          .then((res: any) => {
            console.log('succeed');
          })
          .catch((e) => {
            console.log('failed', e);
          });
      }
    };
  };

  useEffect(() => {
    fetchDocument();
  }, []);

  return <div id="excel" style={{ height: '100%' }}></div>;
};

export default Excel;
