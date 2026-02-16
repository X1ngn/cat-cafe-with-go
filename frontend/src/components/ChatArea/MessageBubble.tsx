import React from 'react';
import { Message } from '@/types';
import { Avatar } from '@/components/common/Avatar';

interface MessageBubbleProps {
  message: Message;
}

export const MessageBubble: React.FC<MessageBubbleProps> = ({ message }) => {
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
        <Avatar color={cat.color || '#ff9966'} size="md" className="rounded-3xl" />
        <div className="cat-message max-w-md">
          <p>{message.content}</p>
        </div>
      </div>
    );
  }

  // user message
  return (
    <div className="flex items-start gap-4 justify-end">
      <div className="user-message max-w-md">
        <p>{message.content}</p>
      </div>
      <Avatar color="#336699" size="md" className="rounded-xl" />
    </div>
  );
};
