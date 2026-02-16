import React from 'react';

interface StatusBadgeProps {
  status: 'idle' | 'busy' | 'offline';
}

const statusConfig = {
  idle: { text: '待命', className: 'status-idle' },
  busy: { text: '工作中', className: 'status-busy' },
  offline: { text: '离线', className: 'bg-gray-300 text-gray-600' },
};

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status }) => {
  const config = statusConfig[status];
  return <span className={`status-badge ${config.className}`}>{config.text}</span>;
};
