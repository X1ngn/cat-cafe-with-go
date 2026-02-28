import React, { useEffect, useState } from 'react';
import { useAppStore } from '@/stores/appStore';
import { chainAPI } from '@/services/api';
import { wsService } from '@/services/websocket';
import { SessionChainStatus, SessionChainItem } from '@/types';

const getProgressColor = (percent: number): string => {
  if (percent < 0.6) return '#22c55e';
  if (percent < 0.8) return '#eab308';
  return '#ef4444';
};

const formatTokens = (tokens: number): string => {
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}k`;
  return String(tokens);
};

const SealedSessionCard: React.FC<{
  session: SessionChainItem;
  expanded: boolean;
  onToggle: () => void;
}> = ({ session, expanded, onToggle }) => {
  const statusLabel = session.status === 'sealed' ? '已压缩' : '压缩中';
  const statusIcon = session.status === 'sealed' ? '📋' : '⏳';

  return (
    <div className="border border-gray-200 rounded-lg overflow-hidden">
      <div
        onClick={onToggle}
        className="flex items-center justify-between py-2.5 px-3 cursor-pointer hover:bg-gray-50 transition-colors"
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onToggle(); } }}
      >
        <div className="flex items-center gap-2">
          <span className="text-sm">{statusIcon}</span>
          <span className="text-sm font-medium">Session #{session.seqNo}</span>
          <span className="text-xs text-gray-400">({statusLabel})</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">
            {session.eventCount} 条 · {formatTokens(session.tokenCount)} tokens
          </span>
          <svg
            className={`w-3.5 h-3.5 text-gray-400 transition-transform ${expanded ? 'rotate-180' : ''}`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </div>
      </div>

      {expanded && session.summary && (
        <div className="border-t border-gray-200 bg-gray-50 p-3">
          {session.sealedAt && (
            <p className="text-xs text-gray-400 mb-2">
              压缩时间: {new Date(session.sealedAt).toLocaleString('zh-CN', {
                month: '2-digit', day: '2-digit',
                hour: '2-digit', minute: '2-digit'
              })}
            </p>
          )}
          <div className="text-sm bg-white border border-gray-200 rounded p-2.5 max-h-40 overflow-y-auto">
            <p className="text-xs text-gray-500 mb-1 font-medium">压缩概述</p>
            <pre className="whitespace-pre-wrap break-words text-xs text-gray-700 leading-relaxed">
              {session.summary}
            </pre>
          </div>
        </div>
      )}

      {expanded && !session.summary && (
        <div className="border-t border-gray-200 bg-gray-50 p-3">
          <p className="text-xs text-gray-400 text-center">
            {session.status === 'compressing' ? '正在压缩中...' : '暂无压缩概述'}
          </p>
        </div>
      )}
    </div>
  );
};

export const SessionChainPanel: React.FC = () => {
  const { currentSession } = useAppStore();
  const [chainStatus, setChainStatus] = useState<SessionChainStatus | null>(null);
  const [expandedSessionId, setExpandedSessionId] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (currentSession) {
      loadChainStatus();

      const unsubscribe = wsService.onChainStatus((status: SessionChainStatus) => {
        setChainStatus(status);
      });

      return () => { unsubscribe(); };
    } else {
      setChainStatus(null);
    }
  }, [currentSession]);

  const loadChainStatus = async () => {
    if (!currentSession) return;
    setLoading(true);
    try {
      const response = await chainAPI.getChainStatus(currentSession.id);
      setChainStatus(response.data);
    } catch {
      // Session Chain 可能未启用，静默处理
      setChainStatus(null);
    } finally {
      setLoading(false);
    }
  };

  const toggleExpand = (sessionId: string) => {
    setExpandedSessionId(expandedSessionId === sessionId ? null : sessionId);
  };

  if (!currentSession || loading) return null;
  if (!chainStatus) return null;

  const sealedSessions = chainStatus.sessions.filter(s => s.status !== 'active');
  const activeSession = chainStatus.activeSession;

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-6">
      <h2 className="text-lg font-bold mb-4">
        Session Chain
        <span className="text-sm font-normal text-gray-400 ml-2">
          ({chainStatus.totalSessions} 个 Session · {chainStatus.totalEvents} 条消息)
        </span>
      </h2>

      <div className="space-y-2">
        {sealedSessions.map((session) => (
          <SealedSessionCard
            key={session.id}
            session={session}
            expanded={expandedSessionId === session.id}
            onToggle={() => toggleExpand(session.id)}
          />
        ))}

        {activeSession && (
          <div className="border border-blue-200 bg-blue-50 rounded-lg p-3">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <span className="text-sm">🟢</span>
                <span className="text-sm font-medium">Session #{activeSession.seqNo}</span>
                <span className="text-xs text-blue-500">(当前)</span>
              </div>
              <span className="text-xs text-gray-500">
                {activeSession.eventCount} 条 · {formatTokens(activeSession.tokenCount)} tokens
              </span>
            </div>

            <div className="w-full bg-gray-200 rounded-full h-2.5 mb-1.5">
              <div
                className="h-2.5 rounded-full transition-all duration-300"
                style={{
                  width: `${Math.min(activeSession.usagePercent * 100, 100)}%`,
                  backgroundColor: getProgressColor(activeSession.usagePercent),
                }}
                role="progressbar"
                aria-valuenow={Math.round(activeSession.usagePercent * 100)}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label={`Token 使用率 ${Math.round(activeSession.usagePercent * 100)}%`}
              />
            </div>

            <div className="flex justify-between text-xs text-gray-500">
              <span>{formatTokens(activeSession.tokenCount)} / {formatTokens(activeSession.maxTokens)} tokens</span>
              <span>{Math.round(activeSession.usagePercent * 100)}%</span>
            </div>
          </div>
        )}

        {!activeSession && sealedSessions.length === 0 && (
          <p className="text-sm text-gray-400 text-center py-3">暂无 Session Chain 数据</p>
        )}
      </div>
    </div>
  );
};
