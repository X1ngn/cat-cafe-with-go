import React, { useState, useRef, useEffect } from 'react';
import { useAppStore } from '@/stores/appStore';
import { messageAPI } from '@/services/api';
import { MentionMenu } from './MentionMenu';

export const MessageInput: React.FC = () => {
  const {
    currentSession,
    inputValue,
    setInputValue,
    addMessage,
    showMentionMenu,
    setShowMentionMenu,
    setMentionQuery,
    setWaitingForReply,
  } = useAppStore();

  const [mentionedCats, setMentionedCats] = useState<string[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setInputValue(value);

    // 检测 @ 符号
    const lastAtIndex = value.lastIndexOf('@');
    const cursorPosition = e.target.selectionStart || 0;

    // 如果光标不在 @ 后面，关闭菜单
    if (lastAtIndex === -1 || cursorPosition <= lastAtIndex) {
      setShowMentionMenu(false);
      return;
    }

    // 如果刚输入 @，显示菜单
    if (lastAtIndex !== -1 && lastAtIndex === value.length - 1) {
      setShowMentionMenu(true);
      setMentionQuery('');
    } else if (lastAtIndex !== -1 && showMentionMenu) {
      const query = value.slice(lastAtIndex + 1, cursorPosition);
      // 如果查询中包含空格或光标移开了 @ 区域，关闭菜单
      if (query.includes(' ') || cursorPosition < lastAtIndex) {
        setShowMentionMenu(false);
      } else {
        setMentionQuery(query);
      }
    }
  };

  const handleSend = async () => {
    if (!inputValue.trim() || !currentSession) return;

    try {
      const response = await messageAPI.sendMessage(
        currentSession.id,
        inputValue,
        mentionedCats
      );
      addMessage(response.data);
      setInputValue('');
      setMentionedCats([]);

      // 发送消息后，设置等待回复状态，触发快速轮询
      setWaitingForReply(true);
    } catch (error) {
      console.error('Failed to send message:', error);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleKeyUp = (e: React.KeyboardEvent<HTMLInputElement>) => {
    // 监听方向键和其他导航键，检查光标位置
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) {
      const target = e.target as HTMLInputElement;
      const cursorPosition = target.selectionStart || 0;
      const lastAtIndex = inputValue.lastIndexOf('@');

      // 如果光标移开了 @ 区域，关闭菜单
      if (lastAtIndex === -1 || cursorPosition <= lastAtIndex) {
        setShowMentionMenu(false);
      }
    }

    // ESC 键关闭菜单
    if (e.key === 'Escape') {
      setShowMentionMenu(false);
    }
  };

  const handleSelectCat = (catId: string, catName: string) => {
    setMentionedCats([...mentionedCats, catId]);
    const lastAtIndex = inputValue.lastIndexOf('@');
    const newValue = inputValue.slice(0, lastAtIndex) + `@${catName} `;
    setInputValue(newValue);
    setShowMentionMenu(false);
    inputRef.current?.focus();
  };

  return (
    <div className="relative px-8 pb-8">
      {showMentionMenu && <MentionMenu onSelect={handleSelectCat} />}

      <div className="bg-white border border-gray-200 rounded-[32px] flex items-center px-6 py-4">
        <input
          ref={inputRef}
          type="text"
          value={inputValue}
          onChange={handleInputChange}
          onKeyPress={handleKeyPress}
          onKeyUp={handleKeyUp}
          placeholder="跟猫猫们说点什么... (@呼叫猫猫)"
          className="flex-1 outline-none text-base"
        />
        <button
          onClick={handleSend}
          disabled={!inputValue.trim()}
          className="w-12 h-12 bg-primary rounded-full flex items-center justify-center hover:bg-opacity-90 transition-colors disabled:opacity-50"
        >
          <span className="text-2xl">🐾</span>
        </button>
      </div>
    </div>
  );
};
