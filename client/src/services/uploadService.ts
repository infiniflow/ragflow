import request from '@/utils/request';
import api from '@/utils/api';

const { upload } = api;

const uploadService = {
  uploadFile: function (file, kb_id) {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('kb_id', kb_id);

    const options = {
      method: 'post',
      data: formData
    };

    return request(upload, options);
  }
};

export default uploadService;
