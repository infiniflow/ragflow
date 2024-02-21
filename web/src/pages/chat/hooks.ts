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
      return dispatch<any>({ type: 'chatModel/setDialog', payload });
    },
    [dispatch],
  );

  return setDialog;
};

export const useFetchDialog = (dialogId: string, visible: boolean): IDialog => {
  const dispatch = useDispatch();
  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  const fetchDialog = useCallback(() => {
    if (dialogId) {
      dispatch({
        type: 'chatModel/getDialog',
        payload: { dialog_id: dialogId },
      });
    }
  }, [dispatch, dialogId]);

  useEffect(() => {
    if (dialogId && visible) {
      fetchDialog();
    }
  }, [dialogId, fetchDialog, visible]);

  return currentDialog;
};

export const useSetCurrentDialog = () => {
  const dispatch = useDispatch();

  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  const setCurrentDialog = useCallback(
    (dialogId: string) => {
      if (dialogId) {
        dispatch({
          type: 'chatModel/setCurrentDialog',
          payload: { id: dialogId },
        });
      }
    },
    [dispatch],
  );

  return { currentDialog, setCurrentDialog };
};

export const useResetCurrentDialog = () => {
  const dispatch = useDispatch();

  const resetCurrentDialog = useCallback(() => {
    dispatch({
      type: 'chatModel/setCurrentDialog',
      payload: {},
    });
  }, [dispatch]);

  return { resetCurrentDialog };
};
