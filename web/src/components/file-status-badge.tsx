// src/pages/dataset/file-logs/file-status-badge.tsx
import { FC } from 'react';

interface StatusBadgeProps {
  status: 'Success' | 'Failed' | 'Running' | 'Pending';
}

const FileStatusBadge: FC<StatusBadgeProps> = ({ status }) => {
  const getStatusColor = () => {
    // #3ba05c  → rgb(59, 160, 92)   // state-success
    // #d8494b  → rgb(216, 73, 75)   // state-error
    // #00beb4  → rgb(0, 190, 180)   // accent-primary
    // #faad14  → rgb(250, 173, 20)  // state-warning
    switch (status) {
      case 'Success':
        return `bg-[rgba(59,160,92,0.1)] text-state-success`;
      case 'Failed':
        return `bg-[rgba(216,73,75,0.1)] text-state-error`;
      case 'Running':
        return `bg-[rgba(0,190,180,0.1)] text-accent-primary`;
      case 'Pending':
        return `bg-[rgba(250,173,20,0.1)] text-state-warning`;
      default:
        return 'bg-gray-500/10 text-white';
    }
  };

  const getBgStatusColor = () => {
    // #3ba05c  → rgb(59, 160, 92)   // state-success
    // #d8494b  → rgb(216, 73, 75)   // state-error
    // #00beb4  → rgb(0, 190, 180)   // accent-primary
    // #faad14  → rgb(250, 173, 20)  // state-warning
    switch (status) {
      case 'Success':
        return `bg-[rgba(59,160,92,1)] text-state-success`;
      case 'Failed':
        return `bg-[rgba(216,73,75,1)] text-state-error`;
      case 'Running':
        return `bg-[rgba(0,190,180,1)] text-accent-primary`;
      case 'Pending':
        return `bg-[rgba(250,173,20,1)] text-state-warning`;
      default:
        return 'bg-gray-500/10 text-white';
    }
  };

  return (
    <span
      className={`inline-flex items-center w-[75px] px-2 py-1 rounded-full text-xs font-medium ${getStatusColor(0.1)}`}
    >
      <div className={`w-1 h-1 mr-1 rounded-full ${getBgStatusColor()}`}></div>
      {status}
    </span>
  );
};

export default FileStatusBadge;
