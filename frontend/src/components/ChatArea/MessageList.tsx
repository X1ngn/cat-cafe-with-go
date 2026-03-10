import React, { useRef, useMemo, forwardRef, useImperativeHandle } from 'react';
import { useAppStore } from '@/stores/appStore';
import { MessageBubble } from './MessageBubble';

export interface MessageListHandle {
  scrollToBottom: () => void;
}

/**
 * 消息列表组件 — 独立订阅 messages，避免输入框状态变化触发重渲染
 * 内部使用 useMemo 缓存渲染结果，只有 messages 引用变化时才重新遍历
 */
export const MessageList = React.memo(forwardRef<MessageListHandle, {
  onScroll: () => void;
  containerRef: React.RefObject<HTMLDivElement>;
}>(({ onScroll, containerRef }, ref) => {
  const messages = useAppStore(state => state.messages);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useImperativeHandle(ref, () => ({
    scrollToBottom: () => {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    },
  }));

  // 缓存消息列表渲染结果，只有 messages 变化时才重新遍历
  const renderedMessages = useMemo(() => {
    return messages.map((message) => (
      <MessageBubble key={message.id} message={message} />
    ));
  }, [messages]);

  return (
    <div
      ref={containerRef}
      onScroll={onScroll}
      className="flex-1 overflow-y-auto overflow-x-hidden px-8 py-10 space-y-6"
    >
      {renderedMessages}
      <div ref={messagesEndRef} />
    </div>
  );
}));

MessageList.displayName = 'MessageList';
