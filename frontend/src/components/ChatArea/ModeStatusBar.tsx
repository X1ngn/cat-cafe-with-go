import React from 'react';

interface ModeStatusBarProps {
  mode: string;
}

const ModeStatusBar: React.FC<ModeStatusBarProps> = ({ mode }) => {
  const getModeDisplay = (mode: string): string => {
    const modeMap: Record<string, string> = {
      free_discussion: '自由讨论模式',
      ipd_dev: 'IPD 开发模式',
    };
    return modeMap[mode] || mode;
  };

  return (
    <div className="mode-status-bar">
      <div className="mode-info">
        <span className="mode-badge">{getModeDisplay(mode)}</span>
      </div>

      <style>{`
        .mode-status-bar {
          display: flex;
          align-items: center;
          gap: 16px;
          padding: 12px 16px;
          background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
          color: white;
          border-radius: 8px;
          margin-bottom: 16px;
        }

        .mode-info {
          display: flex;
          align-items: center;
          gap: 8px;
        }

        .mode-badge {
          padding: 4px 12px;
          border-radius: 12px;
          font-size: 12px;
          font-weight: 600;
          background: rgba(255, 255, 255, 0.2);
          backdrop-filter: blur(10px);
        }
      `}</style>
    </div>
  );
};

export default ModeStatusBar;
