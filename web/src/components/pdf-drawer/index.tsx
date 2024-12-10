import { IModalProps } from '@/interfaces/common';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { Drawer } from 'antd';
import DocumentPreviewer from '../pdf-previewer';

interface IProps extends IModalProps<any> {
  documentId: string;
  chunk: IChunk | IReferenceChunk;
}

export const PdfDrawer = ({
  visible = false,
  hideModal,
  documentId,
  chunk,
}: IProps) => {
  return (
    <Drawer
      title="Document Previewer"
      onClose={hideModal}
      open={visible}
      width={'50vw'}
    >
      <DocumentPreviewer
        documentId={documentId}
        chunk={chunk}
        visible={visible}
      ></DocumentPreviewer>
    </Drawer>
  );
};

export default PdfDrawer;
