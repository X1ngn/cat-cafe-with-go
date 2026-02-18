import { Message, CallHistory } from '@/types';

type WSMessageType = 'message' | 'history' | 'stats' | 'cats';

interface WSMessage {
  type: WSMessageType;
  sessionId?: string;
  data: any;
  timestamp: string;
}

type MessageHandler = (message: Message) => void;
type HistoryHandler = (history: CallHistory[]) => void;

export class WebSocketService {
  private ws: WebSocket | null = null;
  private sessionId: string | null = null;
  private reconnectTimer: NodeJS.Timeout | null = null;
  private messageHandlers: Set<MessageHandler> = new Set();
  private historyHandlers: Set<HistoryHandler> = new Set();
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;

  connect(sessionId: string) {
    if (this.ws && this.sessionId === sessionId) {
      return; // 已连接到该会话
    }

    this.disconnect();
    this.sessionId = sessionId;
    this.reconnectAttempts = 0;
    this.createConnection();
  }

  private createConnection() {
    if (!this.sessionId) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.hostname;
    const port = '8080'; // API 端口
    const wsUrl = `${protocol}//${host}:${port}/api/sessions/${this.sessionId}/ws`;

    console.log('[WS] 连接到:', wsUrl);

    try {
      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        console.log('[WS] 连接已建立');
        this.reconnectAttempts = 0;
      };

      this.ws.onmessage = (event) => {
        try {
          const wsMessage: WSMessage = JSON.parse(event.data);
          this.handleMessage(wsMessage);
        } catch (error) {
          console.error('[WS] 解析消息失败:', error);
        }
      };

      this.ws.onerror = (error) => {
        console.error('[WS] 连接错误:', error);
      };

      this.ws.onclose = () => {
        console.log('[WS] 连接已关闭');
        this.ws = null;
        this.scheduleReconnect();
      };
    } catch (error) {
      console.error('[WS] 创建连接失败:', error);
      this.scheduleReconnect();
    }
  }

  private scheduleReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('[WS] 达到最大重连次数，停止重连');
      return;
    }

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }

    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
    console.log(`[WS] ${delay}ms 后尝试重连 (${this.reconnectAttempts + 1}/${this.maxReconnectAttempts})`);

    this.reconnectTimer = setTimeout(() => {
      this.reconnectAttempts++;
      this.createConnection();
    }, delay);
  }

  private handleMessage(wsMessage: WSMessage) {
    console.log('[WS] 收到消息:', wsMessage.type);

    switch (wsMessage.type) {
      case 'message':
        this.messageHandlers.forEach(handler => handler(wsMessage.data));
        break;
      case 'history':
        this.historyHandlers.forEach(handler => handler(wsMessage.data));
        break;
      default:
        console.warn('[WS] 未知消息类型:', wsMessage.type);
    }
  }

  onMessage(handler: MessageHandler) {
    this.messageHandlers.add(handler);
    return () => this.messageHandlers.delete(handler);
  }

  onHistory(handler: HistoryHandler) {
    this.historyHandlers.add(handler);
    return () => this.historyHandlers.delete(handler);
  }

  disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    this.sessionId = null;
    this.reconnectAttempts = 0;
  }

  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }
}

// 单例
export const wsService = new WebSocketService();
