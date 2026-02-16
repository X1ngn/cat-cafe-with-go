import React, { useEffect, useRef } from 'react';
import { useAppStore } from '@/stores/appStore';
import { MessageBubble } from './MessageBubble';
import { MessageInput } from './MessageInput';
import { messageAPI } from '@/services/api';

export const ChatArea: React.FC = () => {
  const { currentSession, messages, setMessages } = useAppStore();
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (currentSession) {
      loadMessages();
    }
  }, [currentSession]);

  useEffect(() => {
    scrollToBottom();
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

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
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
      {/* 消息列表 */}
      <div className="flex-1 overflow-y-auto px-8 py-10 space-y-6">
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
