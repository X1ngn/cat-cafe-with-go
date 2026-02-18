import React, { useEffect, useRef } from 'react';
import { useAppStore } from '@/stores/appStore';
import { MessageBubble } from './MessageBubble';
import { MessageInput } from './MessageInput';
import ModeStatusBar from './ModeStatusBar';
import { messageAPI, modeAPI } from '@/services/api';
import { wsService } from '@/services/websocket';
import { Message } from '@/types';

export const ChatArea: React.FC = () => {
  const { currentSession, messages, setMessages, sessionMode, setSessionMode } = useAppStore();
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const isUserScrollingRef = useRef<boolean>(false);

  useEffect(() => {
    if (currentSession) {
      loadMessages();
      loadSessionMode();

      // 连接 WebSocket
      wsService.connect(currentSession.id);

      // 监听新消息
      const unsubscribeMessage = wsService.onMessage((message: Message) => {
        console.log('[ChatArea] 收到新消息:', message);
        // 检查消息是否已存在，避免重复
        const currentMessages = useAppStore.getState().messages;
        const exists = currentMessages.some(m => m.id === message.id);
        if (!exists) {
          setMessages([...currentMessages, message]);
        }
      });

      // 清理函数
      return () => {
        unsubscribeMessage();
      };
    }
  }, [currentSession]);

  useEffect(() => {
    // 只有在用户没有主动滚动时才自动滚动到底部
    if (!isUserScrollingRef.current) {
      scrollToBottom();
    }
  }, [messages]);

  const loadMessages = async () => {
    if (!currentSession) return;
    try {
      const response = await messageAPI.getMessages(currentSession.id);
      setMessages(response.data);
    } catch (error) {
      console.error('Failed to load messages:', error);
    }
  };

  const loadSessionMode = async () => {
    if (!currentSession) return;
    try {
      const response = await modeAPI.getSessionMode(currentSession.id);
      setSessionMode(response.data);
    } catch (error) {
      console.error('Failed to load session mode:', error);
    }
  };

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  const handleScroll = () => {
    if (!messagesContainerRef.current) return;

    const { scrollTop, scrollHeight, clientHeight } = messagesContainerRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50; // 50px 容差

    // 如果用户不在底部，标记为正在滚动
    isUserScrollingRef.current = !isAtBottom;
  };

  if (!currentSession) {
    return (
      <div className="flex-1 flex items-center justify-center bg-white">
        <p className="text-gray-400 text-lg">选择或创建一个会话开始聊天</p>
      </div>
    );
  }

  return (
    <div className="flex-1 bg-white flex flex-col">
      {/* 模式状态栏 */}
      {sessionMode && (
        <div className="px-8 pt-4">
          <ModeStatusBar mode={sessionMode.mode} />
        </div>
      )}

      {/* 消息列表 */}
      <div
        ref={messagesContainerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto px-8 py-10 space-y-6"
      >
        {messages.map((message) => (
          <MessageBubble key={message.id} message={message} />
        ))}
        <div ref={messagesEndRef} />
      </div>

      {/* 输入区域 */}
      <MessageInput />
    </div>
  );
};
