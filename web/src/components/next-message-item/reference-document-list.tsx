import { Card, CardContent } from '@/components/ui/card';
import { Docagg } from '@/interfaces/database/chat';
import { middleEllipsis } from '@/utils/common-util';
import FileIcon from '../file-icon';
import NewDocumentLink from '../new-document-link';

export function ReferenceDocumentList({ list }: { list: Docagg[] }) {
  return (
    <section className="flex gap-3 flex-wrap">
      {list.map((item) => (
        <Card key={item.doc_id}>
          <CardContent className="p-2 space-x-2">
            <FileIcon id={item.doc_id} name={item.doc_name}></FileIcon>
            <NewDocumentLink
              documentId={item.doc_id}
              documentName={item.doc_name}
              prefix="document"
              link={item.url}
              className="text-text-sub-title-invert"
            >
              {middleEllipsis(item.doc_name)}
            </NewDocumentLink>
          </CardContent>
        </Card>
      ))}
    </section>
  );
}
