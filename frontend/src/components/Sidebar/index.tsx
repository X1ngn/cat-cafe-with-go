import React from 'react';
import { useAppStore } from '@/stores/appStore';
import { SessionCard } from './SessionCard';
import { sessionAPI } from '@/services/api';

export const Sidebar: React.FC = () => {
  const { sessions, currentSession, setCurrentSession, addSession } = useAppStore();

  const handleNewSession = async () => {
    try {
      const response = await sessionAPI.createSession();
      addSession(response.data);
      setCurrentSession(response.data);
    } catch (error) {
      console.error('Failed to create session:', error);
    }
  };

  return (
    <div className="w-[280px] h-screen bg-white border-r border-gray-200 flex flex-col">
      {/* Logo */}
      <div className="p-6">
        <h1 className="text-2xl font-bold">猫猫咖啡屋</h1>
      </div>

      {/* 新建对话按钮 */}
      <div className="px-6 mb-4">
        <button
          onClick={handleNewSession}
          className="w-full bg-primary text-white py-3 rounded-lg flex items-center justify-center gap-2 hover:bg-opacity-90 transition-colors"
        >
          <span>🐾</span>
          <span className="font-medium">新建对话</span>
        </button>
      </div>

      {/* 会话列表 */}
      <div className="flex-1 overflow-y-auto px-6 space-y-4">
        {sessions.map((session) => (
          <SessionCard
            key={session.id}
            session={session}
            isActive={currentSession?.id === session.id}
            onClick={() => setCurrentSession(session)}
          />
        ))}
      </div>
    </div>
  );
};
