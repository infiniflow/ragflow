import { Spin } from 'antd';
import FileError from '../file-error';

import { useFetchDocx } from '../hooks';
import styles from './index.less';

const Docx = ({ filePath }: { filePath: string }) => {
  const { succeed, containerRef, error } = useFetchDocx(filePath);

  return (
    <>
      {succeed ? (
        <section className={styles.docxViewerWrapper}>
          <div id="docx" ref={containerRef} className={styles.box}>
            <Spin />
          </div>
        </section>
      ) : (
        <FileError>{error}</FileError>
      )}
    </>
  );
};

export default Docx;
