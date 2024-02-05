import api from '@/utils/api';
import request from '@/utils/request';

const { upload } = api;

const uploadService = {
  uploadFile: function (file: any, kb_id: string) {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('kb_id', kb_id);

    const options = {
      method: 'post',
      data: formData,
    };

    return request(upload, options);
  },
};

export default uploadService;
