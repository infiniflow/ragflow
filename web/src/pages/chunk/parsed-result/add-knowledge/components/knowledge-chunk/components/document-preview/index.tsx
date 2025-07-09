import { memo } from 'react';

import CSVFileViewer from './csv-preview';
import { DocPreviewer } from './doc-preview';
import { ExcelCsvPreviewer } from './excel-preview';
import { ImagePreviewer } from './image-preview';
import styles from './index.less';
import PdfPreviewer, { IProps } from './pdf-preview';
import { PptPreviewer } from './ppt-preview';
import { TxtPreviewer } from './txt-preview';

type PreviewProps = {
  fileType: string;
  className?: string;
};
const Preview = ({
  fileType,
  className,
  highlights,
  setWidthAndHeight,
}: PreviewProps & Partial<IProps>) => {
  return (
    <>
      {fileType === 'pdf' && highlights && setWidthAndHeight && (
        <section className={styles.documentPreview}>
          <PdfPreviewer
            highlights={highlights}
            setWidthAndHeight={setWidthAndHeight}
          ></PdfPreviewer>
        </section>
      )}
      {['doc', 'docx'].indexOf(fileType) > -1 && (
        <section>
          <DocPreviewer className={className} />
        </section>
      )}
      {['txt', 'md'].indexOf(fileType) > -1 && (
        <section>
          <TxtPreviewer className={className} />
        </section>
      )}
      {['visual'].indexOf(fileType) > -1 && (
        <section>
          <ImagePreviewer className={className} />
        </section>
      )}
      {['pptx'].indexOf(fileType) > -1 && (
        <section>
          <PptPreviewer className={className} />
        </section>
      )}
      {['xlsx'].indexOf(fileType) > -1 && (
        <section>
          <ExcelCsvPreviewer className={className} />
        </section>
      )}
      {['csv'].indexOf(fileType) > -1 && (
        <section>
          <CSVFileViewer className={className} />
        </section>
      )}
    </>
  );
};
export default memo(Preview);
