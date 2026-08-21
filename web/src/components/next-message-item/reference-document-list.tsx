/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
            document_url: selectedDocument.url,
          }}
        ></PdfDrawer>
      )}
    </section>
  );
}
