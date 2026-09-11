/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
// src/pages/dataset/file-logs/file-status-badge.tsx
import {
  RunningStatus,
  RunningStatusOld,
} from '@/pages/dataset/dataset/constant';
import { FC } from 'react';
/**
 * params: status: 0 not run yet 1 running, 2 cancel, 3 success, 4 fail
 */
interface StatusBadgeProps {
  // status: 'Success' | 'Failed' | 'Running' | 'Pending';
  status: RunningStatus | RunningStatusOld;
  name?: string;
  className?: string;
}

const FileStatusBadge: FC<StatusBadgeProps> = ({ status, name, className }) => {
  const getStatusColor = () => {
    // Aligned with RunningStatusMap colors in src/pages/dataset/dataset/constant.ts
    // so the badge matches the status dot in the document list.
    // #3ba05c  → rgb(59, 160, 92)   // state-success
    // #d8494b  → rgb(216, 73, 75)   // state-error
    // #00beb4  → rgb(0, 190, 180)   // accent-primary
    // #faad14  → rgb(250, 173, 20)  // state-warning
    // #5c96c8  → rgb(92, 150, 200)  // team-member
    switch (status) {
      case RunningStatus.DONE:
      case RunningStatusOld.DONE:
        return `bg-state-success/10 text-state-success`;
      case RunningStatus.FAIL:
      case RunningStatusOld.FAIL:
        return `bg-state-error/10 text-state-error`;
      case RunningStatus.RUNNING:
      case RunningStatusOld.RUNNING:
        return `bg-[rgba(92,150,200,0.1)] text-team-member`;
      case RunningStatus.UNSTART:
      case RunningStatusOld.UNSTART:
        return `bg-accent-primary/10 text-accent-primary`;
      case RunningStatus.CANCEL:
      case RunningStatusOld.CANCEL:
      case RunningStatus.QUEUED:
        return `bg-state-warning/10 text-state-warning`;
      default:
        return 'bg-gray-500/10 text-text-secondary';
    }
  };

  const getBgStatusColor = () => {
    // Aligned with RunningStatusMap colors in src/pages/dataset/dataset/constant.ts
    // so the badge matches the status dot in the document list.
    switch (status) {
      case RunningStatus.DONE:
      case RunningStatusOld.DONE:
        return `bg-state-success text-state-success`;
      case RunningStatus.FAIL:
      case RunningStatusOld.FAIL:
        return `bg-state-error text-state-error`;
      case RunningStatus.RUNNING:
      case RunningStatusOld.RUNNING:
        return `bg-team-member text-team-member`;
      case RunningStatus.UNSTART:
      case RunningStatusOld.UNSTART:
        return `bg-accent-primary text-accent-primary`;
      case RunningStatus.CANCEL:
      case RunningStatusOld.CANCEL:
      case RunningStatus.QUEUED:
        return `bg-state-warning text-state-warning`;
      default:
        return `bg-[rgba(117,120,122,1)] text-text-secondary`;
    }
  };

  return (
    <span
      className={`inline-flex items-center w-[75px] px-2 py-1 rounded-full text-xs font-medium ${getStatusColor()} ${className}`}
    >
      <div className={`w-1 h-1 mr-1 rounded-full ${getBgStatusColor()}`}></div>
      {name || ''}
    </span>
  );
};

export default FileStatusBadge;
