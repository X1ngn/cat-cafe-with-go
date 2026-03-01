import React, { useEffect, useState, useRef } from 'react';
import { hindsightAPI } from '@/services/api';
import { HindsightHealth } from '@/types';

const statusConfig = {
  connected: { label: '正常', icon: '●', className: 'text-green-500' },
  unreachable: { label: '不可用', icon: '○', className: 'text-red-500' },
  disabled: { label: '未启用', icon: '◌', className: 'text-gray-400' },
};

const POLL_INTERVAL = 30000;

export const HindsightPanel: React.FC = () => {
  const [health, setHealth] = useState<HindsightHealth | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval>>();

  const checkHealth = async () => {
    try {
      const response = await hindsightAPI.getHealth();
      setHealth(response.data);
    } catch {
      setHealth({ status: 'unreachable' });
    }
  };

  useEffect(() => {
    checkHealth();
    timerRef.current = setInterval(checkHealth, POLL_INTERVAL);
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  if (!health) return null;

  const config = statusConfig[health.status];

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-6">
      <h2 className="text-lg font-bold mb-4">🧠 长期记忆 (Hindsight)</h2>
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-sm text-gray-500">状态</span>
          <span className={`text-sm font-medium ${config.className}`}>
            {config.icon} {config.label}
          </span>
        </div>
        {health.base_url && (
          <div className="flex items-center justify-between">
            <span className="text-sm text-gray-500">地址</span>
            <span className="text-sm font-mono text-gray-600">
              {health.base_url.replace(/^https?:\/\//, '')}
            </span>
          </div>
        )}
      </div>
    </div>
  );
};
