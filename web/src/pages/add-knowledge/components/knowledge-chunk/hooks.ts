import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { useSelector } from 'umi';

export const useSelectDocumentInfo = () => {
  const documentInfo: IKnowledgeFile = useSelector(
    (state: any) => state.chunkModel.documentInfo,
  );
  return documentInfo;
};
