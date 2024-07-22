import { ChunkModelState } from '@/pages/add-knowledge/components/knowledge-chunk/model';
import { KFModelState } from '@/pages/add-knowledge/components/knowledge-file/model';
import { TestingModelState } from '@/pages/add-knowledge/components/knowledge-testing/model';
import { kAModelState } from '@/pages/add-knowledge/model';
import { ChatModelState } from '@/pages/chat/model';
import { FileManagerModelState } from '@/pages/file-manager/model';
import { LoginModelState } from '@/pages/login/model';
import { SettingModelState } from '@/pages/user-setting/model';

declare module 'lodash';

function useSelector<TState = RootState, TSelected = unknown>(
  selector: (state: TState) => TSelected,
  equalityFn?: (left: TSelected, right: TSelected) => boolean,
): TSelected;

export interface RootState {
  // loading: Loading;
  fileManager: FileManagerModelState;
  chatModel: ChatModelState;
  loginModel: LoginModelState;
  settingModel: SettingModelState;
  kFModel: KFModelState;
  kAModel: kAModelState;
  chunkModel: ChunkModelState;
  testingModel: TestingModelState;
}

declare global {
  type Nullable<T> = T | null;
}

declare module 'umi' {
  export { useSelector };
}
