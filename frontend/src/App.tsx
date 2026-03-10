import { useEffect } from 'react';
import { Sidebar } from './components/Sidebar';
import { ChatArea } from './components/ChatArea';
import { StatusBar } from './components/StatusBar';
import { useAppStore } from './stores/appStore';
import { sessionAPI } from './services/api';
import { wsService } from './services/websocket';

function App() {
  const { setSessions, updateSession } = useAppStore();

  const loadSessions = async () => {
    try {
      const response = await sessionAPI.getSessions();
      setSessions(response.data);
    } catch (error) {
      console.error('Failed to load sessions:', error);
    }
  };

  useEffect(() => {
    loadSessions();

    // 全局订阅 session 元数据更新（不依赖当前连接的 session）
    const unsubSessionUpdate = wsService.onSessionUpdated((data) => {
      updateSession(data.id, {
        summary: data.summary,
        updatedAt: new Date(data.updatedAt),
        messageCount: data.messageCount,
      });
    });

    // 断线重连后全量刷新 sessions，补偿断线期间丢失的 session_updated 事件
    const unsubReconnect = wsService.onReconnect(() => {
      loadSessions();
    });

    return () => {
      unsubSessionUpdate();
      unsubReconnect();
    };
  }, []);

  return (
    <div className="flex h-screen bg-bg-cream">
      <Sidebar />
      <ChatArea />
      <StatusBar />
    </div>
  );
}

export default App;
