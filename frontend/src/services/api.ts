import axios from 'axios';
import { Cat, Message, Session, MessageStats, CallHistory, ModeInfo, SessionMode, Workspace, SessionChainStatus } from '@/types';

const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
});

export const sessionAPI = {
  // 获取所有会话列表
  getSessions: () => api.get<Session[]>('/sessions'),

  // 创建新会话
  createSession: (workspaceId?: string, name?: string) =>
    api.post<Session>('/sessions', { workspace_id: workspaceId, name }),

  // 获取会话详情
  getSession: (sessionId: string) => api.get<Session>(`/sessions/${sessionId}`),

  // 更新会话名称
  updateSession: (sessionId: string, name: string) =>
    api.put<Session>(`/sessions/${sessionId}`, { name }),

  // 删除会话
  deleteSession: (sessionId: string) => api.delete(`/sessions/${sessionId}`),
};

export const messageAPI = {
  // 获取会话的消息列表
  getMessages: (sessionId: string, page = 1, limit = 50) =>
    api.get<Message[]>(`/sessions/${sessionId}/messages`, { params: { page, limit } }),

  // 发送消息
  sendMessage: (sessionId: string, content: string, mentionedCats?: string[]) =>
    api.post<Message>(`/sessions/${sessionId}/messages`, { content, mentionedCats }),

  // 获取消息统计
  getMessageStats: (sessionId: string) =>
    api.get<MessageStats>(`/sessions/${sessionId}/stats`),
};

export const catAPI = {
  // 获取所有猫猫列表
  getCats: () => api.get<Cat[]>('/cats'),

  // 获取猫猫状态
  getCatStatus: (catId: string) => api.get<Cat>(`/cats/${catId}`),

  // 获取可用的猫猫（待命状态）
  getAvailableCats: () => api.get<Cat[]>('/cats/available'),
};

export const historyAPI = {
  // 获取调用历史
  getCallHistory: (sessionId: string) =>
    api.get<CallHistory[]>(`/sessions/${sessionId}/history`),
};

export const modeAPI = {
  // 获取所有可用模式
  getModes: () => api.get<ModeInfo[]>('/modes'),

  // 获取会话当前模式状态
  getSessionMode: (sessionId: string) =>
    api.get<SessionMode>(`/sessions/${sessionId}/mode`),

  // 切换会话模式
  switchMode: (sessionId: string, mode: string, config?: Record<string, any>) =>
    api.put<SessionMode>(`/sessions/${sessionId}/mode`, { mode, modeConfig: config || {} }),
};

export const workspaceAPI = {
  // 获取所有工作区
  getWorkspaces: () => api.get<Workspace[]>('/workspaces'),

  // 创建工作区
  createWorkspace: (path: string, type: 'self' | 'external') =>
    api.post<Workspace>('/workspaces', { path, type }),

  // 获取工作区详情
  getWorkspace: (workspaceId: string) => api.get<Workspace>(`/workspaces/${workspaceId}`),

  // 更新工作区
  updateWorkspace: (workspaceId: string, data: Partial<Workspace>) =>
    api.put<Workspace>(`/workspaces/${workspaceId}`, data),

  // 删除工作区
  deleteWorkspace: (workspaceId: string) => api.delete(`/workspaces/${workspaceId}`),
};

export const chainAPI = {
  // 获取 Session Chain 状态
  getChainStatus: (sessionId: string) =>
    api.get<SessionChainStatus>(`/sessions/${sessionId}/chain-status`),
};

export default api;
