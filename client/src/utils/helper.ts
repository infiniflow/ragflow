import { Modal, Form } from 'antd';
import router from 'umi/router';

export function getMonospaceLength(content = '') {
  let total = 0;
  for (let c of content) {
    if (/[\x00-\xff]/.test(c)) {
      total += 0.5;
    } else {
      total += 1;
    }
  }
  return total;
}

export const pickRecordFormList = function (list = [], ids = [], key = 'id') {
  let results = [];
  for (let id of ids) {
    let find = list.find(record => record[key] === id);
    if (find != null) {
      results.push(find);
    }
  }
  return results;
};

export function extractFieldsValues(fields) {
  let results = {};
  Object.keys(fields).forEach(key => {
    results[key] = fields[key]?.value;
  });
  return results;
}

export function getTableData(response) {
  const { code, data } = response;
  const { count = 0, list = [] } = data || {};
  if (code === 0) {
    return {
      total: count,
      list
    };
  } else {
    return {
      total: 0,
      list: []
    };
  }
}

export const getReportData = response => {
  const { code, data } = response;
  const { rows, total, aggregation = {} } = data || {};
  if (code === 0 && rows.length > 0) {
    return {
      total,
      list: [{ name: '合计', ...aggregation }, ...rows]
    };
  } else {
    return {
      total: 0,
      list: []
    };
  }
};

/**
 * 大数字转换，将大额数字转换为万、千万、亿等
 * @param value 数字值
 */
export const bigNumberTransform = value => {
  if (!value || parseInt(value) < 1000) {
    return value;
  }
  const newValue = ['', '', ''];
  let fr = 1000;
  let num = 3;
  let text1 = '';
  let fm = 1;
  while (value / fr >= 1) {
    fr *= 10;
    num += 1;
    // console.log('数字', value / fr, 'num:', num)
  }
  if (num <= 4) {
    // 千
    newValue[0] = parseInt(value / 1000) + '';
    newValue[1] = '千';
  } else if (num <= 8) {
    // 万
    text1 = parseInt(num - 4) / 3 > 1 ? '千万' : '万';
    // tslint:disable-next-line:no-shadowed-variable
    fm = text1 === '万' ? 10000 : 10000000;
    if (value % fm === 0) {
      newValue[0] = parseInt(value / fm) + '';
    } else {
      newValue[0] = parseFloat(value / fm).toFixed(1) + '';
    }
    newValue[1] = text1;
  } else if (num <= 16) {
    // 亿
    text1 = (num - 8) / 3 > 1 ? '千亿' : '亿';
    text1 = (num - 8) / 4 > 1 ? '万亿' : text1;
    text1 = (num - 8) / 7 > 1 ? '千万亿' : text1;
    // tslint:disable-next-line:no-shadowed-variable
    fm = 1;
    if (text1 === '亿') {
      fm = 100000000;
    } else if (text1 === '千亿') {
      fm = 100000000000;
    } else if (text1 === '万亿') {
      fm = 1000000000000;
    } else if (text1 === '千万亿') {
      fm = 1000000000000000;
    }
    if (value % fm === 0) {
      newValue[0] = parseInt(value / fm) + '';
    } else {
      newValue[0] = parseFloat(value / fm).toFixed(1) + '';
    }
    newValue[1] = text1;
  }
  if (value < 1000) {
    newValue[0] = value + '';
    newValue[1] = '';
  }
  return newValue.join('');
};

export const handleCancel = route => {
  Modal.confirm({
    title: '确认返回？',
    content: '当前处于编辑状态，返回无法保存当前已编辑内容',
    okText: '确认',
    cancelText: '取消',
    onOk() {
      if (route) {
        router.push(route);
      } else {
        router.goBack();
      }
    },
    onCancel() {}
  });
};

export function createFormData(values) {
  let formData = {};
  Object.keys(values || {}).forEach(fieldName => {
    formData[fieldName] = Form.createFormField({
      value: values[fieldName]
    });
  });
  return formData;
}

/**
 * 格式化字符串，超出一定字数后转化为...
 * @param {String}  text   String 文本原文
 * @param {Number}  limit  Number 文本转换的阈值
 * @return {String}       String 转化后的文本
 */
const getShortString = (text, limit) => {
  let _temp = text || '';
  const _limit = parseInt(limit, 10);
  if (_temp.length > _limit) {
    _temp = _temp.slice(0, _limit) + '...';
  }
  return _temp;
};

export const FormatString = {
  getShortString
};

export function GetQueryString(payload = {}) {
  let newPayload = {};
  Object.keys(payload)
    .sort()
    .forEach(key => {
      newPayload[key] = payload[key];
    });
  return JSON.stringify(newPayload);
}

export function ParseQueryString(queryStr) {
  let payload;
  try {
    payload = JSON.parse(queryStr);
  } catch (e) {
    payload = {};
  }
  return payload;
}

export function GetUrlQueryString(params = {}) {
  return Object.keys(params)
    .map(k => {
      if (Array.isArray(params[k])) {
        return params[k].map(v => `${encodeURIComponent(k)}[]=${v}`).join('&');
      } else {
        return encodeURIComponent(k) + '=' + encodeURIComponent(params[k]);
      }
    })
    .join('&');
}

export function IsSameProperty(record, property) {
  let isSame = true;
  for (let key in property) {
    if (record[key] !== property[key]) {
      isSame = false;
      break;
    }
  }
  return isSame;
}

export function getFormContainer(ref) {
  if (ref && ref.current && ref.current.closest) {
    return ref.current.closest('form');
  } else {
    return document.getElementById('form-container');
  }
}

export const toTransformSize = size => {
  size = parseInt(size, 10);
  if (typeof size !== 'number' || size === 0 || !size) return '-';

  if (size < 1024) return `${size} B`;

  const sizeKB = Math.floor(size / 1024);
  if (sizeKB < 1024) return `${sizeKB} KB`;

  const sizeMB = Math.floor(sizeKB / 1024);
  if (sizeMB < 1024) return `${sizeMB} MB`;

  const sizeGB = Math.floor(sizeMB / 1024);
  return `${sizeGB} GB`;
};
