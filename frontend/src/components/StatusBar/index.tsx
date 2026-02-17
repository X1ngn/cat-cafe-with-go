import React, { useEffect, useState } from 'react';
import { useAppStore } from '@/stores/appStore';
import { catAPI, messageAPI, historyAPI } from '@/services/api';
import { Avatar } from '@/components/common/Avatar';
import { StatusBadge } from '@/components/common/StatusBadge';
import { MessageStats, CallHistory } from '@/types';

export const StatusBar: React.FC = () => {
  const { cats, setCats, currentSession } = useAppStore();
  const [stats, setStats] = useState<MessageStats>({ totalMessages: 0, catMessages: 0 });
  const [history, setHistory] = useState<CallHistory[]>([]);

  useEffect(() => {
    loadCats();
  }, []);

  useEffect(() => {
    if (currentSession) {
      loadStats();
      loadHistory();
    }
  }, [currentSession]);

  const loadCats = async () => {
    try {
      const response = await catAPI.getCats();
      setCats(response.data);
    } catch (error) {
      console.error('Failed to load cats:', error);
    }
  };

  const loadStats = async () => {
    if (!currentSession) return;
    try {
      const response = await messageAPI.getMessageStats(currentSession.id);
      setStats(response.data);
    } catch (error) {
      console.error('Failed to load stats:', error);
    }
  };

  const loadHistory = async () => {
    if (!currentSession) return;
    try {
      const response = await historyAPI.getCallHistory(currentSession.id);
      // 去重：只保留每只猫猫的第一次调用记录
      const uniqueHistory = response.data.reduce((acc: CallHistory[], item: CallHistory) => {
        if (!acc.find(h => h.catId === item.catId)) {
          acc.push(item);
        }
        return acc;
      }, []);
      setHistory(uniqueHistory);
    } catch (error) {
      console.error('Failed to load history:', error);
    }
  };

  return (
    <div className="w-[480px] h-screen bg-gray-50 overflow-y-auto p-6 space-y-6">
      {/* 猫猫状态区 */}
      <div className="bg-white border border-gray-200 rounded-xl p-6">
        <h2 className="text-lg font-bold mb-4">猫猫们的状态</h2>
        <div className="space-y-4">
          {cats.map((cat) => (
            <div key={cat.id} className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Avatar color={cat.color} size="sm" className="rounded-2xl" avatar={cat.avatar} />
                <span className="font-medium">{cat.name}</span>
              </div>
              <StatusBadge status={cat.status} />
            </div>
          ))}
        </div>
      </div>

      {/* 消息统计区 */}
      <div className="bg-white border border-gray-200 rounded-xl p-6">
        <h2 className="text-lg font-bold mb-4">消息统计</h2>
        <div className="flex gap-4">
          <div className="flex-1 bg-gray-100 rounded-lg p-4">
            <p className="text-xs text-gray-500 mb-1">总消息数</p>
            <p className="text-xl font-bold">{stats.totalMessages.toLocaleString()}</p>
          </div>
          <div className="flex-1 bg-gray-100 rounded-lg p-4">
            <p className="text-xs text-gray-500 mb-1">猫猫消息数</p>
            <p className="text-xl font-bold">{stats.catMessages.toLocaleString()}</p>
          </div>
        </div>
      </div>

      {/* 调用历史区 */}
      <div className="bg-white border border-gray-200 rounded-xl p-6">
        <h2 className="text-lg font-bold mb-4">调用历史</h2>
        <div className="space-y-2">
          {history.map((item, index) => (
            <div
              key={index}
              className="flex items-center justify-between py-2 border-b border-gray-100 last:border-0"
            >
              <span className="text-sm">{item.catName}</span>
              <span className="text-sm text-gray-500">sess_...{item.sessionId.slice(-4)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
