import showDeleteConfirm from '@/components/deleting-confirm';
import { IDialog } from '@/interfaces/database/chat';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSelector } from 'umi';
import { v4 as uuid } from 'uuid';
import { VariableTableDataType } from './interface';

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

export const useSelectPromptConfigParameters = (): VariableTableDataType[] => {
  const currentDialog: IDialog = useSelector(
    (state: any) => state.chatModel.currentDialog,
  );

  const finalParameters: VariableTableDataType[] = useMemo(() => {
    const parameters = currentDialog?.prompt_config?.parameters ?? [];
    if (!currentDialog.id) {
      // The newly created chat has a default parameter
      return [{ key: uuid(), variable: 'knowledge', optional: false }];
    }
    return parameters.map((x) => ({
      key: uuid(),
      variable: x.key,
      optional: x.optional,
    }));
  }, [currentDialog]);

  return finalParameters;
};

export const useRemoveDialog = () => {
  const dispatch = useDispatch();

  const removeDocument = (dialogIds: Array<string>) => () => {
    return dispatch({
      type: 'chatModel/removeDialog',
      payload: {
        dialog_ids: dialogIds,
      },
    });
  };

  const onRemoveDialog = (dialogIds: Array<string>) => {
    showDeleteConfirm({ onOk: removeDocument(dialogIds) });
  };

  return { onRemoveDialog };
};
