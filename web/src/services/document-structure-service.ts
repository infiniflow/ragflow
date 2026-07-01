import api from '@/utils/api';
import request from '@/utils/next-request';

export const getDocumentStructureGraph = (
  datasetId: string,
  documentId: string,
) => request.get(api.documentStructureGraph(datasetId, documentId));

const documentStructureService = {
  getDocumentStructureGraph,
};

export default documentStructureService;
