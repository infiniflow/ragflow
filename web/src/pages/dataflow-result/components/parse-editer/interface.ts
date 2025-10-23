import { CheckedState } from '@radix-ui/react-checkbox';
import { ChunkTextMode } from '../../constant';
import { ComponentParams, IChunk } from '../../interface';
import { parserKeyMap } from './json-parser';

export interface FormatPreserveEditorProps {
  initialValue: {
    key: keyof typeof parserKeyMap | 'text' | 'html';
    type: string;
    value: Array<{ [key: string]: string }>;
    params: ComponentParams;
  };
  onSave: (value: any) => void;
  className?: string;
  isSelect?: boolean;
  isDelete?: boolean;
  isChunck?: boolean;
  handleCheckboxClick?: (id: string | number, checked: boolean) => void;
  selectedChunkIds?: string[];
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
}

export type IJsonContainerProps = {
  initialValue: {
    key: keyof typeof parserKeyMap;
    type: string;
    value: {
      [key: string]: string;
    }[];
    params: ComponentParams;
  };
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  selectedChunkIds: string[] | undefined;
  unescapeNewlines: (text: string) => string;
  escapeNewlines: (text: string) => string;
  onSave: (data: {
    value: {
      text: string;
    }[];
    key: string;
    type: string;
  }) => void;
  className?: string;
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
};

export type IObjContainerProps = {
  initialValue: {
    key: string;
    type: string;
    value: string;
    params: ComponentParams;
  };
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  unescapeNewlines: (text: string) => string;
  escapeNewlines: (text: string) => string;
  onSave: (data: { value: string; key: string; type: string }) => void;
  className?: string;
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
};
