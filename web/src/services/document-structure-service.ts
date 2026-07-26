import api from '@/utils/api';
import request from '@/utils/next-request';

export const getDocumentStructureGraph = (
  datasetId: string,
  documentId: string,
) => request.get(api.documentStructureGraph(datasetId, documentId));

export const deleteDocumentStructureGraph = (
  datasetId: string,
  documentId: string,
  templateId: string,
) =>
  request.delete(api.documentStructureGraph(datasetId, documentId), {
    data: { template_id: templateId },
  });

const documentStructureService = {
  getDocumentStructureGraph,
  deleteDocumentStructureGraph,
};

export default documentStructureService;
