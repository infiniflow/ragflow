export type FilterType = {
  id: string;
  label: string;
  count: number;
};

export function groupListByType<T extends Record<string, any>>(
  list: T[],
  idField: string,
  labelField: string,
) {
  const fileTypeList: FilterType[] = [];
  list.forEach((x) => {
    const item = fileTypeList.find((y) => y.id === x[idField]);
    if (!item) {
      fileTypeList.push({ id: x[idField], label: x[labelField], count: 1 });
    } else {
      item.count += 1;
    }
  });

  return fileTypeList;
}

export function groupListByArray<T extends Record<string, any>>(
  list: T[],
  idField: string,
) {
  const fileTypeList: FilterType[] = [];
  list.forEach((x) => {
    if (Array.isArray(x[idField])) {
      x[idField].forEach((j) => {
        const item = fileTypeList.find((i) => i.id === j);
        if (!item) {
          fileTypeList.push({ id: j, label: j, count: 1 });
        } else {
          item.count += 1;
        }
      });
    }
  });

  return fileTypeList;
}

export function buildOwnersFilter<T extends Record<string, any>>(
  list: T[],
  nickName?: string,
) {
  const owners = groupListByType(list, 'tenant_id', nickName || 'nickname');

  return { field: 'owner', list: owners, label: 'Owner' };
}
