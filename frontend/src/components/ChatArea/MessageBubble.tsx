import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Message } from '@/types';
import { Avatar } from '@/components/common/Avatar';

// 格式化消息时间戳为 YYYY-MM-DD HH:MM
const formatMessageTime = (timestamp: Date): string => {
  const date = new Date(timestamp);
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  return `${year}-${month}-${day} ${hours}:${minutes}`;
};

// 自定义 ReactMarkdown 组件：为代码块和表格添加独立滚动容器
const markdownComponents = {
  // 代码块 <pre> — 用滚动容器包裹
  pre: ({ children, ...props }: React.HTMLAttributes<HTMLPreElement>) => (
    <div className="code-scroll-wrapper">
      <pre {...props}>{children}</pre>
    </div>
  ),
  // 表格 <table> — 用滚动容器包裹
  table: ({ children, ...props }: React.TableHTMLAttributes<HTMLTableElement>) => (
    <div className="table-scroll-wrapper">
      <table {...props}>{children}</table>
    </div>
  ),
};

interface MessageBubbleProps {
  message: Message;
}

export const MessageBubble: React.FC<MessageBubbleProps> = React.memo(({ message }) => {
  if (message.type === 'system') {
    return (
      <div className="flex justify-center">
        <span className="system-message">{message.content}</span>
      </div>
    );
  }

  if (message.type === 'cat') {
    const cat = message.sender as { id: string; name: string; avatar: string; color?: string };
    return (
      <div className="flex items-start gap-4">
        <Avatar color={cat.color || '#ff9966'} size="md" className="rounded-3xl" avatar={cat.avatar} />
        <div className="min-w-0 flex-1">
          <div className="cat-message max-w-2xl prose prose-sm break-words">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
              {message.content}
            </ReactMarkdown>
          </div>
          <p className="text-xs text-gray-400 mt-1 ml-1">
            {formatMessageTime(message.timestamp)}
          </p>
        </div>
      </div>
    );
  }

  // user message
  const user = message.sender as { id: string; name: string; avatar: string };
  return (
    <div className="flex items-start gap-4 justify-end">
      <div className="min-w-0 flex-1">
        <div className="user-message max-w-2xl prose prose-sm break-words ml-auto">
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
            {message.content}
          </ReactMarkdown>
        </div>
        <p className="text-xs text-gray-400 mt-1 mr-1 text-right">
          {formatMessageTime(message.timestamp)}
        </p>
      </div>
      <Avatar color="#336699" size="md" className="rounded-xl" avatar={user?.avatar} />
    </div>
  );
}, (prevProps, nextProps) => {
  // 比较所有影响渲染的字段：id、content、type、sender、timestamp
  return prevProps.message.id === nextProps.message.id &&
         prevProps.message.content === nextProps.message.content &&
         prevProps.message.type === nextProps.message.type &&
         prevProps.message.sender === nextProps.message.sender &&
         new Date(prevProps.message.timestamp).getTime() === new Date(nextProps.message.timestamp).getTime();
});
