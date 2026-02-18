import { create } from 'zustand';
import { Cat, Message, Session, SessionMode } from '@/types';

interface AppState {
  // 当前会话
  currentSession: Session | null;
  setCurrentSession: (session: Session | null) => void;

  // 会话列表
  sessions: Session[];
  setSessions: (sessions: Session[]) => void;
  addSession: (session: Session) => void;
  updateSession: (sessionId: string, updates: Partial<Session>) => void;
  removeSession: (sessionId: string) => void;

  // 消息列表
  messages: Message[];
  setMessages: (messages: Message[]) => void;
  addMessage: (message: Message) => void;

  // 猫猫列表
  cats: Cat[];
  setCats: (cats: Cat[]) => void;

  // 输入框内容
  inputValue: string;
  setInputValue: (value: string) => void;

  // Mention 菜单状态
  showMentionMenu: boolean;
  setShowMentionMenu: (show: boolean) => void;
  mentionQuery: string;
  setMentionQuery: (query: string) => void;

  // 等待回复状态
  waitingForReply: boolean;
  setWaitingForReply: (waiting: boolean) => void;

  // 会话模式状态
  sessionMode: SessionMode | null;
  setSessionMode: (mode: SessionMode | null) => void;
}

export const useAppStore = create<AppState>((set) => ({
  currentSession: null,
  setCurrentSession: (session) => set({ currentSession: session }),

  sessions: [],
  setSessions: (sessions) => set({ sessions }),
  addSession: (session) => set((state) => ({ sessions: [session, ...state.sessions] })),
  updateSession: (sessionId, updates) => set((state) => ({
    sessions: state.sessions.map(s => s.id === sessionId ? { ...s, ...updates } : s),
    currentSession: state.currentSession?.id === sessionId
      ? { ...state.currentSession, ...updates }
      : state.currentSession
  })),
  removeSession: (sessionId) => set((state) => ({
    sessions: state.sessions.filter(s => s.id !== sessionId)
  })),

  messages: [],
  setMessages: (messages) => set({ messages }),
  addMessage: (message) => set((state) => ({ messages: [...state.messages, message] })),

  cats: [],
  setCats: (cats) => set({ cats }),

  inputValue: '',
  setInputValue: (value) => set({ inputValue: value }),

  showMentionMenu: false,
  setShowMentionMenu: (show) => set({ showMentionMenu: show }),
  mentionQuery: '',
  setMentionQuery: (query) => set({ mentionQuery: query }),

  waitingForReply: false,
  setWaitingForReply: (waiting) => set({ waitingForReply: waiting }),

  sessionMode: null,
  setSessionMode: (mode) => set({ sessionMode: mode }),
}));
