import { memo } from 'react';

import CSVFileViewer from './csv-preview';
import { DocPreviewer } from './doc-preview';
import { ExcelCsvPreviewer } from './excel-preview';
import { ImagePreviewer } from './image-preview';
import styles from './index.less';
import PdfPreviewer, { IProps } from './pdf-preview';
import { PptPreviewer } from './ppt-preview';
import { TxtPreviewer } from './txt-preview';
import { VideoPreviewer } from './video-preview';

type PreviewProps = {
  fileType: string;
  className?: string;
  url: string;
};
const Preview = ({
  fileType,
  className,
  highlights,
  setWidthAndHeight,
  url,
}: PreviewProps & Partial<IProps>) => {
  return (
    <>
      {fileType === 'pdf' && highlights && setWidthAndHeight && (
        <section className={styles.documentPreview}>
          <PdfPreviewer
            highlights={highlights}
            setWidthAndHeight={setWidthAndHeight}
            url={url}
          ></PdfPreviewer>
        </section>
      )}
      {['doc', 'docx'].indexOf(fileType) > -1 && (
        <section>
          <DocPreviewer className={className} url={url} />
        </section>
      )}
      {['txt', 'md'].indexOf(fileType) > -1 && (
        <section>
          <TxtPreviewer className={className} url={url} />
        </section>
      )}
      {['jpg', 'png', 'gif', 'jpeg', 'svg', 'bmp', 'ico', 'tif'].indexOf(
        fileType,
      ) > -1 && (
        <section>
          <ImagePreviewer className={className} url={url} />
        </section>
      )}
      {[
        'mp4',
        'avi',
        'mov',
        'mkv',
        'wmv',
        'flv',
        'mpeg',
        'mpg',
        'asf',
        'rm',
        'rmvb',
      ].indexOf(fileType) > -1 && (
        <section>
          <VideoPreviewer className={className} url={url} />
        </section>
      )}
      {['pptx'].indexOf(fileType) > -1 && (
        <section>
          <PptPreviewer className={className} url={url} />
        </section>
      )}
      {['xlsx'].indexOf(fileType) > -1 && (
        <section>
          <ExcelCsvPreviewer className={className} url={url} />
        </section>
      )}
      {['csv'].indexOf(fileType) > -1 && (
        <section>
          <CSVFileViewer className={className} url={url} />
        </section>
      )}
    </>
  );
};
export default memo(Preview);
