import { IModalProps } from '@/interfaces/common';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { cn } from '@/lib/utils';
import DocumentPreviewer from '../pdf-previewer';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '../ui/sheet';

interface IProps extends IModalProps<any> {
  documentId: string;
  chunk: IChunk | IReferenceChunk;
  width?: string | number;
  height?: string | number;
}

export const PdfSheet = ({
  visible = false,
  hideModal,
  documentId,
  chunk,
  width = '50vw',
  height,
}: IProps) => {
  return (
    <Sheet open onOpenChange={hideModal}>
      <SheetContent
        className={cn(`max-w-full`)}
        style={{
          width: width,
          height: height ? height : undefined,
        }}
      >
        <SheetHeader>
          <SheetTitle>Document Previewer</SheetTitle>
        </SheetHeader>
        <DocumentPreviewer
          documentId={documentId}
          chunk={chunk}
          visible={visible}
        ></DocumentPreviewer>
      </SheetContent>
    </Sheet>
  );
};

export default PdfSheet;
