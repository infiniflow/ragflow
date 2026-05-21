import { Card, CardContent } from '@/components/ui/card';
import { useSetModalState } from '@/hooks/common-hooks';
import { Docagg } from '@/interfaces/database/chat';
import PdfDrawer from '@/pages/next-search/document-preview-modal';
import { middleEllipsis } from '@/utils/common-util';
import { useState } from 'react';
import FileIcon from '../file-icon';

export function ReferenceDocumentList({ list }: { list: Docagg[] }) {
  const { visible, showModal, hideModal } = useSetModalState();
  const [selectedDocument, setSelectedDocument] = useState<Docagg>();
  return (
    <section className="flex gap-3 flex-wrap">
      {list.map((item) => (
        <Card key={item.doc_id}>
          <CardContent
            className="flex items-center p-2 space-x-2 cursor-pointer"
            onClick={() => {
              setSelectedDocument(item);
              showModal();
            }}
          >
            <FileIcon id={item.doc_id} name={item.doc_name}></FileIcon>
            {/* <NewDocumentLink
              documentId={item.doc_id}
              documentName={item.doc_name}
              prefix="document"
              link={item.url}
              className="text-text-sub-title-invert"
            >
              {middleEllipsis(item.doc_name)}
            </NewDocumentLink> */}
            <div className="text-text-sub-title-invert">
              {middleEllipsis(item.doc_name)}
            </div>
          </CardContent>
        </Card>
      ))}
      {visible && selectedDocument && (
        <PdfDrawer
          visible={visible}
          hideModal={hideModal}
          documentId={selectedDocument.doc_id}
          chunk={{
            document_name: selectedDocument.doc_name,
          }}
        ></PdfDrawer>
      )}
    </section>
  );
}
