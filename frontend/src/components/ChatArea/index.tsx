import React, { useEffect, useRef, useCallback } from 'react';
import { useAppStore } from '@/stores/appStore';
import { MessageList, MessageListHandle } from './MessageList';
import { MessageInput } from './MessageInput';
import ModeStatusBar from './ModeStatusBar';
import { messageAPI, modeAPI } from '@/services/api';
import { wsService } from '@/services/websocket';
import { Message } from '@/types';

export const ChatArea: React.FC = () => {
  const currentSession = useAppStore(state => state.currentSession);
  const setMessages = useAppStore(state => state.setMessages);
  const addMessageIfNotExists = useAppStore(state => state.addMessageIfNotExists);
  const sessionMode = useAppStore(state => state.sessionMode);
  const setSessionMode = useAppStore(state => state.setSessionMode);
  const messageListRef = useRef<MessageListHandle>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const isUserScrollingRef = useRef<boolean>(false);
  const messagesLoadedRef = useRef<boolean>(false);
  const pendingWsMessagesRef = useRef<Message[]>([]);

  // 使用 session ID 字符串作为依赖，避免对象引用变化触发无意义的重载
  const currentSessionId = currentSession?.id;

  useEffect(() => {
    if (!currentSessionId) return;

    messagesLoadedRef.current = false;
    pendingWsMessagesRef.current = [];

    loadMessages(currentSessionId);
    loadSessionMode(currentSessionId);

    wsService.connect(currentSessionId);

    const unsubscribeMessage = wsService.onMessage((message: Message) => {
      console.log('[ChatArea] received new message:', message);

      if (!messagesLoadedRef.current) {
        pendingWsMessagesRef.current.push(message);
        return;
      }

      addMessageIfNotExists(message);

      if (!isUserScrollingRef.current) {
        requestAnimationFrame(() => {
          messageListRef.current?.scrollToBottom();
        });
      }
    });

    // 订阅 WS 重连事件，重连后重新拉取消息，避免断连期间消息丢失
    const unsubscribeReconnect = wsService.onReconnect(() => {
      console.log('[ChatArea] WS reconnected, reloading messages');
      loadMessages(currentSessionId);
    });

    return () => {
      unsubscribeMessage();
      unsubscribeReconnect();
    };
  }, [currentSessionId]);

  const loadMessages = async (sessionId?: string) => {
    const targetSessionId = sessionId || currentSessionId;
    if (!targetSessionId) return;
    try {
      const response = await messageAPI.getMessages(targetSessionId);
      setMessages(response.data);
      messagesLoadedRef.current = true;

      if (pendingWsMessagesRef.current.length > 0) {
        const pending = pendingWsMessagesRef.current;
        pendingWsMessagesRef.current = [];
        pending.forEach(msg => addMessageIfNotExists(msg));
      }

      requestAnimationFrame(() => {
        messageListRef.current?.scrollToBottom();
      });
    } catch (error) {
      console.error('Failed to load messages:', error);
      messagesLoadedRef.current = true;
      const pending = pendingWsMessagesRef.current;
      pendingWsMessagesRef.current = [];
      pending.forEach(msg => addMessageIfNotExists(msg));
    }
  };

  const loadSessionMode = async (sessionId?: string) => {
    const targetSessionId = sessionId || currentSessionId;
    if (!targetSessionId) return;
    try {
      const response = await modeAPI.getSessionMode(targetSessionId);
      setSessionMode(response.data);
    } catch (error) {
      console.error('Failed to load session mode:', error);
    }
  };

  const handleScroll = useCallback(() => {
    if (!messagesContainerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = messagesContainerRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    isUserScrollingRef.current = !isAtBottom;
  }, []);

  if (!currentSession) {
    return (
      <div className="flex-1 flex items-center justify-center bg-white">
        <p className="text-gray-400 text-lg">选择或创建一个会话开始聊天</p>
      </div>
    );
  }

  return (
    <div className="flex-1 bg-white flex flex-col">
      {currentSession.workspacePath && (
        <div className="px-8 pt-4 pb-2">
          <div className="flex items-center gap-2 text-sm text-gray-600 bg-blue-50 border border-blue-200 rounded-lg px-4 py-2">
            <span className="text-blue-600">📁</span>
            <span className="font-medium text-blue-700">工作区:</span>
            <span className="truncate" title={currentSession.workspacePath}>
              {currentSession.workspacePath}
            </span>
          </div>
        </div>
      )}

      {sessionMode && (
        <div className="px-8 pt-2">
          <ModeStatusBar mode={sessionMode.mode} />
        </div>
      )}

      <MessageList
        ref={messageListRef}
        containerRef={messagesContainerRef}
        onScroll={handleScroll}
      />

      <MessageInput />
    </div>
  );
};
