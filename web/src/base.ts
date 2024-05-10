import isObject from 'lodash/isObject';
import { DvaModel } from 'umi';
import { BaseState } from './interfaces/common';

type State = Record<string, any>;
type DvaModelKey<T> = keyof DvaModel<T>;

export const modelExtend = <T>(
  baseModel: Partial<DvaModel<any>>,
  extendModel: DvaModel<any>,
): DvaModel<T> => {
  return Object.keys(extendModel).reduce<DvaModel<T>>((pre, cur) => {
    const baseValue = baseModel[cur as DvaModelKey<State>];
    const value = extendModel[cur as DvaModelKey<State>];

    if (isObject(value) && isObject(baseValue) && typeof value !== 'string') {
      const key = cur as Exclude<DvaModelKey<State>, 'namespace'>;

      pre[key] = {
        ...baseValue,
        ...value,
      } as any;
    } else {
      pre[cur as DvaModelKey<State>] = value as any;
    }

    return pre;
  }, {} as DvaModel<T>);
};

export const paginationModel: Partial<DvaModel<BaseState>> = {
  state: {
    searchString: '',
    pagination: {
      total: 0,
      current: 1,
      pageSize: 10,
    },
  },
  reducers: {
    setSearchString(state, { payload }) {
      return { ...state, searchString: payload };
    },
    setPagination(state, { payload }) {
      return { ...state, pagination: { ...state.pagination, ...payload } };
    },
  },
};
