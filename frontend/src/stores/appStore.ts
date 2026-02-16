import { create } from 'zustand';
import { Cat, Message, Session } from '@/types';

interface AppState {
  // 当前会话
  currentSession: Session | null;
  setCurrentSession: (session: Session | null) => void;

  // 会话列表
  sessions: Session[];
  setSessions: (sessions: Session[]) => void;
  addSession: (session: Session) => void;

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
}

export const useAppStore = create<AppState>((set) => ({
  currentSession: null,
  setCurrentSession: (session) => set({ currentSession: session }),

  sessions: [],
  setSessions: (sessions) => set({ sessions }),
  addSession: (session) => set((state) => ({ sessions: [session, ...state.sessions] })),

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
}));
