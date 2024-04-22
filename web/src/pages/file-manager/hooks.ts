import { useTranslate } from '@/hooks/commonHooks';
import { useFetchFileList } from '@/hooks/fileManagerHooks';
import { Pagination } from '@/interfaces/common';
import { PaginationProps } from 'antd';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSelector } from 'umi';

export const useFetchDocumentListOnMount = () => {
  const fetchDocumentList = useFetchFileList();

  const dispatch = useDispatch();

  useEffect(() => {
    fetchDocumentList();
  }, [dispatch, fetchDocumentList]);

  return { fetchDocumentList };
};

export const useGetPagination = (fetchDocumentList: () => void) => {
  const dispatch = useDispatch();
  const kFModel = useSelector((state: any) => state.kFModel);
  const { t } = useTranslate('common');

  const setPagination = useCallback(
    (pageNumber = 1, pageSize?: number) => {
      const pagination: Pagination = {
        current: pageNumber,
      } as Pagination;
      if (pageSize) {
        pagination.pageSize = pageSize;
      }
      dispatch({
        type: 'kFModel/setPagination',
        payload: pagination,
      });
    },
    [dispatch],
  );

  const onPageChange: PaginationProps['onChange'] = useCallback(
    (pageNumber: number, pageSize: number) => {
      setPagination(pageNumber, pageSize);
      fetchDocumentList();
    },
    [fetchDocumentList, setPagination],
  );

  const pagination: PaginationProps = useMemo(() => {
    return {
      showQuickJumper: true,
      total: kFModel.total,
      showSizeChanger: true,
      current: kFModel.pagination.current,
      pageSize: kFModel.pagination.pageSize,
      pageSizeOptions: [1, 2, 10, 20, 50, 100],
      onChange: onPageChange,
      showTotal: (total) => `${t('total')} ${total}`,
    };
  }, [kFModel, onPageChange, t]);

  return {
    pagination,
    setPagination,
    total: kFModel.total,
    searchString: kFModel.searchString,
  };
};

export const useHandleSearchChange = (setPagination: () => void) => {
  const dispatch = useDispatch();

  const throttledGetDocumentList = useCallback(() => {
    dispatch({
      type: 'kFModel/throttledGetDocumentList',
    });
  }, [dispatch]);

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      const value = e.target.value;
      dispatch({ type: 'kFModel/setSearchString', payload: value });
      setPagination();
      throttledGetDocumentList();
    },
    [setPagination, throttledGetDocumentList, dispatch],
  );

  return { handleInputChange };
};
