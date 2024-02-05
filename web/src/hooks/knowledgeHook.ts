import showDeleteConfirm from '@/components/deleting-confirm';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { useDispatch, useSearchParams, useSelector } from 'umi';

export const useKnowledgeBaseId = (): string => {
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id');

  return knowledgeBaseId || '';
};

export const useDeleteDocumentById = (): {
  removeDocument: (documentId: string) => Promise<number>;
} => {
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();

  const removeDocument = (documentId: string) => () => {
    return dispatch({
      type: 'kFModel/document_rm',
      payload: {
        doc_id: documentId,
        kb_id: knowledgeBaseId,
      },
    });
  };

  const onRmDocument = (documentId: string): Promise<number> => {
    return showDeleteConfirm({ onOk: removeDocument(documentId) });
  };

  return {
    removeDocument: onRmDocument,
  };
};

export const useGetDocumentDefaultParser = (knowledgeBaseId: string) => {
  const data: IKnowledge[] = useSelector(
    (state: any) => state.knowledgeModel.data,
  );

  const item = data.find((x) => x.id === knowledgeBaseId);

  return {
    defaultParserId: item?.parser_id ?? '',
    parserConfig: item?.parser_config ?? '',
  };
};
