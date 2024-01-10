const UpladFile = props => {
  let { xhr, options } = props;
  const { file, url, uploadData, headers, callback, getProgress } = options;

  var form = new FormData(); // FormData 对象
  form.append('file', file); // 文件对象
  for (let key in uploadData) {
    if (uploadData.hasOwnProperty(key)) {
      form.append(key, uploadData[key]);
    }
  }
  xhr = new XMLHttpRequest(); // XMLHttpRequest 对象
  xhr.open('post', url, true); //post方式，url为服务器请求地址，true 该参数规定请求是否异步处理。
  for (let key in headers) {
    if (headers.hasOwnProperty(key)) {
      xhr.setRequestHeader(key, headers[key]);
    }
  }
  xhr.onload = evt => callback(true, JSON.parse(evt.target.response)); //请求完成
  xhr.onerror = evt => callback(false, JSON.parse(evt.target.response)); //请求失败
  xhr.upload.onprogress = evt => {
    if (evt.lengthComputable) {
      const rate = Math.round((evt.loaded / evt.total) * 100);
      if (getProgress) getProgress(file.uid, rate);
    }
  };
  xhr.send(form); //开
};

const cancleUploadFile = xhr => {
  xhr.abort();
};

const upladFileProgress = props => {
  const { file, url, uploadData, headers, callback, getProgress } = props;
  let xhr;
  UpladFile({ xhr, options: { file, url, uploadData, headers, callback, getProgress } });
};

export default upladFileProgress;
