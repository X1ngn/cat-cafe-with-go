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
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);

  useEffect(() => {
    loadCats();
  }, []);

  useEffect(() => {
    if (currentSession) {
      loadStats();
      loadHistory();

      // 设置定时刷新调用历史（每3秒）
      const interval = setInterval(() => {
        loadHistory();
      }, 3000);

      return () => clearInterval(interval);
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
      setHistory(response.data);
    } catch (error) {
      console.error('Failed to load history:', error);
    }
  };

  const toggleExpand = (index: number) => {
    setExpandedIndex(expandedIndex === index ? null : index);
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
        <h2 className="text-lg font-bold mb-4">调用历史 ({history.length})</h2>
        <div className="space-y-2 max-h-96 overflow-y-auto">
          {history.length === 0 ? (
            <p className="text-sm text-gray-400 text-center py-4">暂无调用记录</p>
          ) : (
            history.map((item, index) => (
              <div
                key={index}
                className="border border-gray-200 rounded-lg overflow-hidden"
              >
                {/* 摘要行 - 可点击 */}
                <div
                  onClick={() => toggleExpand(index)}
                  className="flex items-center justify-between py-3 px-4 cursor-pointer hover:bg-gray-50 transition-colors"
                >
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{item.catName}</span>
                    <span className="text-xs text-gray-400">
                      {new Date(item.timestamp).toLocaleTimeString('zh-CN', {
                        hour: '2-digit',
                        minute: '2-digit',
                        second: '2-digit'
                      })}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-500">sess_...{item.sessionId.slice(-4)}</span>
                    <svg
                      className={`w-4 h-4 text-gray-400 transition-transform ${
                        expandedIndex === index ? 'rotate-180' : ''
                      }`}
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                    </svg>
                  </div>
                </div>

                {/* 详情面板 - 可展开 */}
                {expandedIndex === index && (
                  <div className="border-t border-gray-200 bg-gray-50 p-4 space-y-3">
                    <div>
                      <p className="text-xs text-gray-500 mb-1">猫猫 ID</p>
                      <p className="text-sm font-mono">{item.catId}</p>
                    </div>
                    <div>
                      <p className="text-xs text-gray-500 mb-1">会话 ID</p>
                      <p className="text-sm font-mono break-all">{item.sessionId}</p>
                    </div>
                    <div>
                      <p className="text-xs text-gray-500 mb-1">调用时间</p>
                      <p className="text-sm">
                        {new Date(item.timestamp).toLocaleString('zh-CN', {
                          year: 'numeric',
                          month: '2-digit',
                          day: '2-digit',
                          hour: '2-digit',
                          minute: '2-digit',
                          second: '2-digit'
                        })}
                      </p>
                    </div>
                    {item.prompt && (
                      <div>
                        <p className="text-xs text-gray-500 mb-1">调用提示词 (Prompt)</p>
                        <div className="text-sm bg-white border border-gray-200 rounded p-2 max-h-32 overflow-y-auto">
                          <pre className="whitespace-pre-wrap break-words text-xs">{item.prompt}</pre>
                        </div>
                      </div>
                    )}
                    {item.response && (
                      <div>
                        <p className="text-xs text-gray-500 mb-1">猫猫回复 (Response)</p>
                        <div className="text-sm bg-white border border-gray-200 rounded p-2 max-h-48 overflow-y-auto">
                          <pre className="whitespace-pre-wrap break-words text-xs">{item.response}</pre>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
