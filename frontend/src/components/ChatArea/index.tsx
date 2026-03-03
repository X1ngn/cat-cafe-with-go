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
  const sessionMode = useAppStore(state => state.sessionMode);
  const setSessionMode = useAppStore(state => state.setSessionMode);
  const messageListRef = useRef<MessageListHandle>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const isUserScrollingRef = useRef<boolean>(false);

  useEffect(() => {
    if (currentSession) {
      loadMessages();
      loadSessionMode();

      wsService.connect(currentSession.id);

      const unsubscribeMessage = wsService.onMessage((message: Message) => {
        console.log('[ChatArea] received new message:', message);
        const currentMessages = useAppStore.getState().messages;
        const exists = currentMessages.some(m => m.id === message.id);
        if (!exists) {
          setMessages([...currentMessages, message]);
          if (!isUserScrollingRef.current) {
            requestAnimationFrame(() => {
              messageListRef.current?.scrollToBottom();
            });
          }
        }
      });

      return () => {
        unsubscribeMessage();
      };
    }
  }, [currentSession]);

  const loadMessages = async () => {
    if (!currentSession) return;
    try {
      const response = await messageAPI.getMessages(currentSession.id);
      setMessages(response.data);
      requestAnimationFrame(() => {
        messageListRef.current?.scrollToBottom();
      });
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
