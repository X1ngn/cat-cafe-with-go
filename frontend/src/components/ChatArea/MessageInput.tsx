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
  } = useAppStore();

  const [mentionedCats, setMentionedCats] = useState<string[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setInputValue(value);

    // 检测 @ 符号
    const lastAtIndex = value.lastIndexOf('@');
    if (lastAtIndex !== -1 && lastAtIndex === value.length - 1) {
      setShowMentionMenu(true);
      setMentionQuery('');
    } else if (lastAtIndex !== -1 && showMentionMenu) {
      const query = value.slice(lastAtIndex + 1);
      if (query.includes(' ')) {
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
