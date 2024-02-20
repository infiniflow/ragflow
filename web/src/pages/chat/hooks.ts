import { IDialog } from '@/interfaces/database/chat';
import { useCallback, useEffect } from 'react';
import { useDispatch, useSelector } from 'umi';

export const useFetchDialogList = () => {
  const dispatch = useDispatch();
  const dialogList: IDialog[] = useSelector(
    (state: any) => state.chatModel.dialogList,
  );

  useEffect(() => {
    dispatch({ type: 'chatModel/listDialog' });
  }, [dispatch]);

  return dialogList;
};

export const useSetDialog = () => {
  const dispatch = useDispatch();

  const setDialog = useCallback(
    (payload: IDialog) => {
      dispatch({ type: 'chatModel/setDialog', payload });
    },
    [dispatch],
  );

  return setDialog;
};
