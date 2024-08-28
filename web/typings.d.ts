import { KFModelState } from '@/pages/add-knowledge/components/knowledge-file/model';

declare module 'lodash';

function useSelector<TState = RootState, TSelected = unknown>(
  selector: (state: TState) => TSelected,
  equalityFn?: (left: TSelected, right: TSelected) => boolean,
): TSelected;

export interface RootState {
  kFModel: KFModelState;
}

declare global {
  type Nullable<T> = T | null;
}

declare module 'umi' {
  export { useSelector };
}
