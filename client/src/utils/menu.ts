import { formatMessage } from 'umi-plugin-react/locale';
import { check } from '@/components/Authorize/getAuth';
import memoize from 'lodash/memoize';

function formatter(data, parentName) {
  if (!data) {
    return null;
  }
  return data
    .map(item => {
      if (!item.name || !item.path) {
        return null;
      }
      let locale = parentName ? `${parentName}.${item.name}` : `menu.${item.name}`;
      const result = {
        ...item,
        locale: formatMessage({
          id: locale,
          defaultMessage: item.name
        })
      };
      if (item.routes) {
        result.children = formatter(item.routes, locale);
      }
      delete result.routes;
      return result;
    })
    .filter(item => item);
}

export const getMenuData = memoize(formatter);

const getSubMenu = item => {
  // doc: add hideChildrenInMenu
  if (item.children && item.children.some(child => child.name)) {
    return {
      ...item,
      children: getAuthorizedMenuData(item.children) // eslint-disable-line
    };
  }
  return item;
};

export const getAuthorizedMenuData = (menuData, currentAuth) => {
  return menuData.map(item => check(item.auth, currentAuth, getSubMenu(item))).filter(item => item);
};

export const getAuthorizeByHeader = (menuData, path) => {
  const formatMenus = menuData.filter(d => d.path === path || path?.includes(d.extraKey));
  return formatMenus;
};
