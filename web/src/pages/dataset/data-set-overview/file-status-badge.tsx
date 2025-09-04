// src/pages/dataset/file-logs/file-status-badge.tsx
import { FC } from 'react';

interface StatusBadgeProps {
  status: 'Success' | 'Failed' | 'Running' | 'Pending';
}

const FileStatusBadge: FC<StatusBadgeProps> = ({ status }) => {
  const getStatusColor = () => {
    switch (status) {
      case 'Success':
        return 'bg-green-500';
      case 'Failed':
        return 'bg-red-500';
      case 'Running':
        return 'bg-blue-500';
      case 'Pending':
        return 'bg-yellow-500';
      default:
        return 'bg-gray-500';
    }
  };

  return (
    <span
      className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium text-white ${getStatusColor()}`}
    >
      {status}
    </span>
  );
};

export default FileStatusBadge;
