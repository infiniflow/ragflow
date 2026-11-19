import { CheckedState } from '@radix-ui/react-checkbox';
import { ChunkTextMode } from '../../constant';
import { ComponentParams, IChunk } from '../../interface';

type ParserResultBase = {
  type: string;
  params: ComponentParams;
};

type SectionResult = ParserResultBase & {
  key: 'json' | 'chunks';
  value: Record<string, string>[];
};

type TextResult = ParserResultBase & {
  key: 'text' | 'html' | 'markdown';
  value: string;
};

export type ParserEditorValue = SectionResult | TextResult;

export interface FormatPreserveEditorProps {
  initialValue: ParserEditorValue;
  onSave: (value: ParserEditorValue) => void;
  className?: string;
  isChunck?: boolean;
  handleCheckboxClick?: (id: string | number, checked: boolean) => void;
  selectedChunkIds?: string[];
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
  newChunkIndex?: number;
}

export type IJsonContainerProps = {
  initialValue: SectionResult;
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  selectedChunkIds: string[] | undefined;
  onSave: (data: SectionResult) => void;
  className?: string;
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
  newChunkIndex?: number;
};

export type IObjContainerProps = {
  initialValue: TextResult;
  isChunck?: boolean;
  onSave: (data: TextResult) => void;
  className?: string;
  textMode?: ChunkTextMode;
  clickChunk: (chunk: IChunk) => void;
  isReadonly: boolean;
};
