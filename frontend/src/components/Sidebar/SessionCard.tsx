import React from 'react';
import { Session } from '@/types';

interface SessionCardProps {
  session: Session;
  isActive: boolean;
  onClick: () => void;
}

export const SessionCard: React.FC<SessionCardProps> = ({ session, isActive, onClick }) => {
  const formatTime = (date: Date) => {
    const now = new Date();
    const diff = now.getTime() - new Date(date).getTime();
    const minutes = Math.floor(diff / 60000);

    if (minutes < 60) return `${minutes}分钟前`;
    if (minutes < 1440) return `${Math.floor(minutes / 60)}小时前`;
    return `${Math.floor(minutes / 1440)}天前`;
  };

  return (
    <div
      onClick={onClick}
      className={isActive ? 'session-card-active' : 'session-card'}
    >
      <h3 className="font-bold text-base mb-2">{session.name}</h3>
      <p className="text-xs text-gray-500 mb-1">{formatTime(session.updatedAt)}</p>
      <p className="text-xs text-gray-600 truncate">{session.summary}</p>
    </div>
  );
};
