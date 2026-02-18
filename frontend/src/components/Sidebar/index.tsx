import React, { useState, useMemo } from 'react';
import { useAppStore } from '@/stores/appStore';
import { SessionCard } from './SessionCard';
import { sessionAPI } from '@/services/api';

export const Sidebar: React.FC = () => {
  const { sessions, currentSession, setCurrentSession, addSession, removeSession } = useAppStore();
  const [searchQuery, setSearchQuery] = useState('');

  const handleNewSession = async () => {
    try {
      const response = await sessionAPI.createSession();
      addSession(response.data);
      setCurrentSession(response.data);
    } catch (error) {
      console.error('Failed to create session:', error);
    }
  };

  const handleDeleteSession = async (sessionId: string) => {
    if (!window.confirm('确定要删除这个对话吗？删除后无法恢复。')) {
      return;
    }

    try {
      await sessionAPI.deleteSession(sessionId);
      removeSession(sessionId);

      // 如果删除的是当前会话，切换到第一个会话
      if (currentSession?.id === sessionId) {
        const remainingSessions = sessions.filter(s => s.id !== sessionId);
        if (remainingSessions.length > 0) {
          setCurrentSession(remainingSessions[0]);
        } else {
          setCurrentSession(null);
        }
      }
    } catch (error) {
      console.error('Failed to delete session:', error);
      alert('删除失败，请重试');
    }
  };

  // 筛选 sessions
  const filteredSessions = useMemo(() => {
    if (!searchQuery.trim()) {
      return sessions;
    }

    const query = searchQuery.toLowerCase();
    return sessions.filter(session =>
      session.name.toLowerCase().includes(query) ||
      (session.summary && session.summary.toLowerCase().includes(query))
    );
  }, [sessions, searchQuery]);

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

      {/* 搜索框 */}
      <div className="px-6 mb-4">
        <div className="relative">
          <input
            type="text"
            placeholder="搜索对话..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full px-4 py-2 pl-10 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
          />
          <svg
            className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        </div>
      </div>

      {/* 会话列表 */}
      <div className="flex-1 overflow-y-auto px-6 space-y-4">
        {filteredSessions.length === 0 ? (
          <div className="text-center text-gray-400 mt-8">
            {searchQuery ? '没有找到匹配的对话' : '暂无对话'}
          </div>
        ) : (
          filteredSessions.map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              isActive={currentSession?.id === session.id}
              onClick={() => setCurrentSession(session)}
              onDelete={() => handleDeleteSession(session.id)}
            />
          ))
        )}
      </div>
    </div>
  );
};
