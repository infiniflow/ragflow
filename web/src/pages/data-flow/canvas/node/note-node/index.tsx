import { INoteNode } from '@/interfaces/database/flow';
import BaseNoteNode from '@/pages/agent/canvas/node/note-node';
import { NodeProps } from '@xyflow/react';
import { memo } from 'react';
import { useWatchFormChange, useWatchNameFormChange } from './use-watch-change';

function NoteNode({ ...props }: NodeProps<INoteNode>) {
  return (
    <BaseNoteNode
      {...props}
      useWatchNoteFormChange={useWatchFormChange}
      useWatchNoteNameFormChange={useWatchNameFormChange}
    ></BaseNoteNode>
  );
}

export default memo(NoteNode);
