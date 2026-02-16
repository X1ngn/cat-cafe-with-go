import React, { useEffect } from 'react';
import { Sidebar } from './components/Sidebar';
import { ChatArea } from './components/ChatArea';
import { StatusBar } from './components/StatusBar';
import { useAppStore } from './stores/appStore';
import { sessionAPI } from './services/api';

function App() {
  const { setSessions } = useAppStore();

  useEffect(() => {
    loadSessions();
  }, []);

  const loadSessions = async () => {
    try {
      const response = await sessionAPI.getSessions();
      setSessions(response.data);
    } catch (error) {
      console.error('Failed to load sessions:', error);
    }
  };

  return (
    <div className="flex h-screen bg-bg-cream">
      <Sidebar />
      <ChatArea />
      <StatusBar />
    </div>
  );
}

export default App;
