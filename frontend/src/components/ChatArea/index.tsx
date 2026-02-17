import React, { useEffect, useRef } from 'react';
import { useAppStore } from '@/stores/appStore';
import { MessageBubble } from './MessageBubble';
import { MessageInput } from './MessageInput';
import { messageAPI } from '@/services/api';

export const ChatArea: React.FC = () => {
  const { currentSession, messages, setMessages, waitingForReply } = useAppStore();
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null);
  const lastMessageCountRef = useRef<number>(0);

  useEffect(() => {
    if (currentSession) {
      loadMessages();
      // 启动轮询
      startPolling();
    }

    // 清理函数：组件卸载或会话切换时停止轮询
    return () => {
      stopPolling();
    };
  }, [currentSession]);

  // 根据 waitingForReply 状态调整轮询频率
  useEffect(() => {
    if (currentSession) {
      startPolling();
    }
  }, [waitingForReply]);

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  const loadMessages = async () => {
    if (!currentSession) return;
    try {
      const response = await messageAPI.getMessages(currentSession.id);
      const newMessages = response.data;

      // 检查是否有新消息
      if (newMessages.length > lastMessageCountRef.current) {
        // 检查最后一条消息是否是猫猫的回复
        const lastMessage = newMessages[newMessages.length - 1];
        if (lastMessage.type === 'cat') {
          // 收到猫猫回复，停止快速轮询
          useAppStore.getState().setWaitingForReply(false);
        }
      }

      lastMessageCountRef.current = newMessages.length;
      setMessages(newMessages);
    } catch (error) {
      console.error('Failed to load messages:', error);
    }
  };

  const startPolling = () => {
    // 清除之前的轮询
    stopPolling();

    // 根据是否等待回复设置不同的轮询间隔
    const interval = waitingForReply ? 1000 : 3000; // 等待回复时 1 秒，否则 3 秒

    // 设置新的轮询
    pollingIntervalRef.current = setInterval(() => {
      loadMessages();
    }, interval);
  };

  const stopPolling = () => {
    if (pollingIntervalRef.current) {
      clearInterval(pollingIntervalRef.current);
      pollingIntervalRef.current = null;
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
