export interface Cat {
  id: string;
  name: string;
  avatar: string;
  color: string;
  status: 'idle' | 'busy' | 'offline';
}

export interface Message {
  id: string;
  type: 'cat' | 'user' | 'system';
  content: string;
  sender?: Cat | { id: string; name: string; avatar: string };
  timestamp: Date;
  sessionId: string;
}

export interface Session {
  id: string;
  name: string;
  summary: string;
  updatedAt: Date;
  messageCount: number;
}

export interface MessageStats {
  totalMessages: number;
  catMessages: number;
}

export interface CallHistory {
  catId: string;
  catName: string;
  sessionId: string;
  timestamp: Date;
  prompt: string;
  response: string;
}

// 模式相关类型
export interface ModeConfig {
  name: string;
  enabled: boolean;
  [key: string]: any;
}

export interface ModeState {
  customState: Record<string, any>;
  lastUpdateTime: string;
}

export interface ModeInfo {
  name: string;
  description: string;
}

export interface SessionMode {
  mode: string;
  description: string;
  config: ModeConfig;
  state: ModeState;
}
