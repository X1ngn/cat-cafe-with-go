import React, { useState, useRef, useEffect } from 'react';
import { useAppStore } from '@/stores/appStore';
import { messageAPI, modeAPI, catAPI } from '@/services/api';
import { MentionMenu } from './MentionMenu';
import { ModeInfo } from '@/types';

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
    sessionMode,
    setSessionMode,
    cats,
    setCats,
  } = useAppStore();

  const [mentionedCats, setMentionedCats] = useState<string[]>([]);
  const [showModeMenu, setShowModeMenu] = useState(false);
  const [availableModes, setAvailableModes] = useState<ModeInfo[]>([]);
  const [loadingMode, setLoadingMode] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const modeMenuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    loadAvailableModes();
    loadCats();
  }, []);

  // 自动调整 textarea 高度
  useEffect(() => {
    const textarea = inputRef.current;
    if (textarea) {
      // 重置高度以获取正确的 scrollHeight
      textarea.style.height = '24px';
      // 设置新高度，最大 200px
      const newHeight = Math.min(textarea.scrollHeight, 200);
      textarea.style.height = `${newHeight}px`;
    }
  }, [inputValue]);

  const loadCats = async () => {
    try {
      const response = await catAPI.getCats();
      setCats(response.data);
    } catch (error) {
      console.error('Failed to load cats:', error);
    }
  };

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (modeMenuRef.current && !modeMenuRef.current.contains(event.target as Node)) {
        setShowModeMenu(false);
      }
    };

    if (showModeMenu) {
      document.addEventListener('mousedown', handleClickOutside);
    }

    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [showModeMenu]);

  const loadAvailableModes = async () => {
    try {
      const response = await modeAPI.getModes();
      setAvailableModes(response.data);
    } catch (error) {
      console.error('Failed to load modes:', error);
    }
  };

  const handleSwitchMode = async (modeName: string) => {
    if (!currentSession || loadingMode) return;

    setLoadingMode(true);
    try {
      const response = await modeAPI.switchMode(currentSession.id, modeName);
      setSessionMode(response.data);
      setShowModeMenu(false);
    } catch (error) {
      console.error('Failed to switch mode:', error);
    } finally {
      setLoadingMode(false);
    }
  };

  const getCurrentModeName = (): string => {
    if (!sessionMode) return '自由讨论';
    const mode = availableModes.find((m) => m.name === sessionMode.mode);
    if (!mode) return sessionMode.mode;

    // 简化模式名称
    if (mode.name === 'free_discussion') return '自由讨论';
    if (mode.name === 'ipd_dev') return 'IPD 开发';
    return mode.description.split('：')[0];
  };

  const getModeIcon = (): string => {
    if (!sessionMode) return '💬';
    if (sessionMode.mode === 'ipd_dev') return '🔧';
    return '💬';
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    console.log('=== handleInputChange ===');
    console.log('新值:', value);
    console.log('旧值:', inputValue);
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

  // 从文本中解析实际提及的猫猫
  const parseMentionedCats = (text: string): string[] => {
    console.log('=== parseMentionedCats 开始 ===');
    console.log('输入文本:', text);
    console.log('文本长度:', text.length);
    console.log('文本字符码:', Array.from(text).map(c => `${c}(${c.charCodeAt(0)})`).join(' '));
    console.log('可用猫猫列表:', cats);

    const mentionedCatIds: string[] = [];

    // 匹配所有 @猫猫名
    cats.forEach((cat) => {
      // 使用更宽松的匹配，支持中文字符
      const regex = new RegExp(`@${cat.name}(?=\\s|$)`, 'g');
      const matches = regex.test(text);
      console.log(`测试 @${cat.name}:`, matches, '正则:', regex.source);
      if (matches) {
        console.log(`✓ 匹配成功，添加猫猫 ID:`, cat.id);
        mentionedCatIds.push(cat.id);
      }
    });

    console.log('最终解析结果:', mentionedCatIds);
    console.log('=== parseMentionedCats 结束 ===');
    return mentionedCatIds;
  };

  const handleSend = async () => {
    if (!inputValue.trim() || !currentSession) return;

    console.log('=== handleSend 开始 ===');
    console.log('当前输入值:', inputValue);

    try {
      // 从实际输入内容中解析 @ 提及的猫猫
      const actualMentionedCats = parseMentionedCats(inputValue);

      console.log('准备发送消息:');
      console.log('  - sessionId:', currentSession.id);
      console.log('  - content:', inputValue);
      console.log('  - mentionedCats:', actualMentionedCats);

      const response = await messageAPI.sendMessage(
        currentSession.id,
        inputValue,
        actualMentionedCats
      );
      // 不再手动添加消息，等待 WebSocket 推送
      // addMessage(response.data);
      setInputValue('');
      setMentionedCats([]);

      // 发送消息后，设置等待回复状态
      setWaitingForReply(true);
      console.log('=== handleSend 完成 ===');
    } catch (error) {
      console.error('Failed to send message:', error);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleKeyUp = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // 监听方向键和其他导航键，检查光标位置
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) {
      const target = e.target as HTMLTextAreaElement;
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

      <div className="flex items-center gap-3">
        {/* 模式切换按钮 */}
        <div className="relative" ref={modeMenuRef}>
          <button
            onClick={() => setShowModeMenu(!showModeMenu)}
            disabled={loadingMode || !currentSession}
            className="w-10 h-10 rounded-full bg-gradient-to-r from-purple-500 to-indigo-500 text-white flex items-center justify-center hover:shadow-lg transition-all disabled:opacity-50 disabled:cursor-not-allowed"
            title={`当前模式: ${getCurrentModeName()}`}
          >
            <span className="text-xl">{getModeIcon()}</span>
          </button>

          {/* 模式选择菜单 */}
          {showModeMenu && (
            <div className="absolute bottom-full left-0 mb-2 bg-white rounded-lg shadow-xl border border-gray-200 min-w-[160px] z-50">
              {availableModes.map((mode) => {
                const modeName = mode.name === 'free_discussion' ? '自由讨论' : mode.name === 'ipd_dev' ? 'IPD 开发' : mode.description.split('：')[0];
                const modeIcon = mode.name === 'ipd_dev' ? '🔧' : '💬';

                return (
                  <button
                    key={mode.name}
                    onClick={() => handleSwitchMode(mode.name)}
                    disabled={loadingMode}
                    className={`w-full text-left px-4 py-3 hover:bg-gray-50 transition-colors first:rounded-t-lg last:rounded-b-lg border-b last:border-b-0 flex items-center gap-2 ${
                      sessionMode?.mode === mode.name ? 'bg-purple-50' : ''
                    }`}
                  >
                    <span className="text-lg">{modeIcon}</span>
                    <span className="font-medium text-gray-900">{modeName}</span>
                    {sessionMode?.mode === mode.name && <span className="ml-auto text-purple-600">✓</span>}
                  </button>
                );
              })}
            </div>
          )}
        </div>

        {/* 输入框容器 */}
        <div className="flex-1 bg-white border border-gray-200 rounded-[32px] flex items-center px-6 py-3">
          {/* 输入框 */}
          <textarea
            ref={inputRef}
            value={inputValue}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            onKeyUp={handleKeyUp}
            placeholder="跟猫猫们说点什么... (@呼叫猫猫)"
            className="flex-1 outline-none text-base resize-none overflow-y-auto"
            rows={1}
            style={{
              minHeight: '24px',
              maxHeight: '200px',
            }}
          />

          {/* 发送按钮 */}
          <button
            onClick={handleSend}
            disabled={!inputValue.trim()}
            className="w-12 h-12 bg-primary rounded-full flex items-center justify-center hover:bg-opacity-90 transition-colors disabled:opacity-50 flex-shrink-0"
          >
            <span className="text-2xl">🐾</span>
          </button>
        </div>
      </div>
    </div>
  );
};
