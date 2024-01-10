/**
 * @param  {String}  url
 * @param  {Boolean} isNoCaseSensitive 是否区分大小写
 * @return {Object}
 */
// import numeral from 'numeral';
import store from '@/utils/persistStore';

export const parseQuery = (url = window.location.search, isNoCaseSensitive: boolean) => {
  return window.g_history.location.query;
  let arr, part;

  const query = {};
  //去掉首位空格
  if (!(url || '').replace(/^\s+|\s+$/, '')) {
    return {};
  }

  url = url.replace(/\S*\?/, '');

  if (url) {
    if (isNoCaseSensitive) {
      url = url.toLocaleLowerCase();
    }

    arr = url.split('&');
    for (let i in arr) {
      part = arr[i].split('=');
      query[part[0]] = decodeURIComponent(part[1]);
    }
  }
  return query;
};

export const parseQueryForPath = url => {
  if (!url) {
    return {};
  }

  let arr, part;

  const query = {};
  //去掉首位空格
  if (!(url || '').replace(/^\s+|\s+$/, '')) {
    return {};
  }

  url = url.replace(/\S*\?/, '');

  if (url) {
    arr = url.split('&');
    for (let i in arr) {
      part = arr[i].split('=');
      query[part[0]] = decodeURIComponent(part[1]);
    }
  }
  return query;
};

export const param = paramObj => {
  let str = [];
  for (let i in paramObj) {
    if (typeof paramObj[i] !== 'undefined') {
      str.push(i + '=' + encodeURIComponent(paramObj[i]));
    }
  }
  return str.join('&');
};

export const addParam = (url, params) => {
  let SEARCH_REG = /\?([^#]*)/,
    HASH_REG = /#(.*)/,
    searchStr;

  url = url || '';
  let search = {},
    searchMatch = url.match(SEARCH_REG);

  if (searchMatch) {
    search = parseQuery(searchMatch[0]);
  }

  //合并当前search参数
  search = Object.assign(search, params);

  searchStr = '?' + param(search);

  //是否存在search
  if (SEARCH_REG.test(url)) {
    url = url.replace(SEARCH_REG, searchStr);
  } else {
    //是否存在hash
    if (HASH_REG.test(url)) {
      url = url.replace(HASH_REG, searchStr + '#' + url.match(HASH_REG)[1]);
    } else {
      url += searchStr;
    }
  }
  return url;
};

const downloadWithIframe = (url: string) => {
  const id = `downloadIframe${new Date().getTime()}`;
  let iframe = document.createElement('iframe');
  iframe.id = id;
  iframe.onload = function () {
    console.log('开始加载');
  };
  document.body.appendChild(iframe);
  iframe.style.cssText = 'display:none';
  const iframeDoc = iframe.contentWindow.document;
  iframeDoc.open(); // 打开流
  iframeDoc.write(`<iframe src="${url}"></iframe>`);
  iframeDoc.write('<script>');
  iframeDoc.write(
    `window.onload = function() { setTimeout(function() {parent.document.body.removeChild(parent.document.getElementById("${id}"))}, 20000)}`
  );
  iframeDoc.write('</script>');
  iframeDoc.close(); // 关闭流
};




export const getReportVersion = () => {
  const erVersion = window.localStorage.getItem('EASY_REPORT_VERSION');
  let version = undefined;
  if (window.location.host === 'adv.martechlab.cn') {
    version = '';
  } else if (erVersion) {
    version = erVersion;
  }
  return version;
};

export const delay = timeout => {
  return new Promise(resolve => {
    setTimeout(resolve, timeout);
  });
};

export const formatRequestUrlByDomainPrefix = url => {
  let prefix = store.domainPrefix || '';
  if (prefix) {
    prefix = `//mp${prefix}.`;
    url = url.slice(2).split('.').slice(1).join('.');
  }
  return `${prefix}${url}`;
};
export const getWidth = () => {
  return { width: window.innerWidth };
};

export default {
  parseQuery,
  downloadWithIframe,
  formatRequestUrlByDomainPrefix,
  getWidth
};
