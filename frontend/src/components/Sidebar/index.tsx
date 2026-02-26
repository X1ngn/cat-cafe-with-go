import React, { useState, useMemo, useEffect } from 'react';
import { useAppStore } from '@/stores/appStore';
import { SessionCard } from './SessionCard';
import { sessionAPI, workspaceAPI } from '@/services/api';
import { Workspace } from '@/types';

export const Sidebar: React.FC = () => {
  const { sessions, currentSession, setCurrentSession, addSession, removeSession, updateSession } = useAppStore();
  const [searchQuery, setSearchQuery] = useState('');
  const [showWorkspaceDialog, setShowWorkspaceDialog] = useState(false);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string>('');
  const [showCreateWorkspace, setShowCreateWorkspace] = useState(false);
  const [newWorkspacePath, setNewWorkspacePath] = useState('');
  const [newWorkspaceType, setNewWorkspaceType] = useState<'self' | 'external'>('external');

  // 加载工作区列表
  useEffect(() => {
    const loadWorkspaces = async () => {
      try {
        const response = await workspaceAPI.getWorkspaces();
        setWorkspaces(response.data);
      } catch (error) {
        console.error('Failed to load workspaces:', error);
      }
    };
    loadWorkspaces();
  }, []);

  const handleNewSession = () => {
    // 始终显示工作区选择对话框
    setShowWorkspaceDialog(true);
  };

  const createSession = async (workspaceId?: string) => {
    try {
      const response = await sessionAPI.createSession(workspaceId);
      addSession(response.data);
      setCurrentSession(response.data);
      setShowWorkspaceDialog(false);
      setSelectedWorkspaceId('');
    } catch (error) {
      console.error('Failed to create session:', error);
    }
  };

  const handleWorkspaceDialogSubmit = () => {
    createSession(selectedWorkspaceId || undefined);
  };

  const handleCreateWorkspace = async () => {
    if (!newWorkspacePath.trim()) {
      alert('请输入工作区路径');
      return;
    }

    try {
      const response = await workspaceAPI.createWorkspace(newWorkspacePath.trim(), newWorkspaceType);
      const newWorkspace = response.data;
      setWorkspaces([...workspaces, newWorkspace]);
      setSelectedWorkspaceId(newWorkspace.id);
      setShowCreateWorkspace(false);
      setNewWorkspacePath('');
      alert('工作区创建成功！');
    } catch (error) {
      console.error('Failed to create workspace:', error);
      alert('创建工作区失败，请检查路径是否有效');
    }
  };

  const handleRenameSession = async (sessionId: string, newName: string) => {
    try {
      const response = await sessionAPI.updateSession(sessionId, newName);
      updateSession(sessionId, response.data);
    } catch (error) {
      console.error('Failed to rename session:', error);
      alert('重命名失败，请重试');
    }
  };

  const handleDeleteSession = async (sessionId: string) => {
    if (!window.confirm('确定要删除这个对话吗？删除后无法恢复。')) {
      return;
    }

    try {
      await sessionAPI.deleteSession(sessionId);
      removeSession(sessionId);

      // 如果删除的是当前会话，切换到第一个会话
      if (currentSession?.id === sessionId) {
        const remainingSessions = sessions.filter(s => s.id !== sessionId);
        if (remainingSessions.length > 0) {
          setCurrentSession(remainingSessions[0]);
        } else {
          setCurrentSession(null);
        }
      }
    } catch (error) {
      console.error('Failed to delete session:', error);
      alert('删除失败，请重试');
    }
  };

  // 筛选 sessions
  const filteredSessions = useMemo(() => {
    if (!searchQuery.trim()) {
      return sessions;
    }

    const query = searchQuery.toLowerCase();
    return sessions.filter(session =>
      session.name.toLowerCase().includes(query) ||
      (session.summary && session.summary.toLowerCase().includes(query))
    );
  }, [sessions, searchQuery]);

  return (
    <div className="w-[280px] h-screen bg-white border-r border-gray-200 flex flex-col">
      {/* Logo */}
      <div className="p-6">
        <h1 className="text-2xl font-bold">猫猫咖啡屋</h1>
      </div>

      {/* 新建对话按钮 */}
      <div className="px-6 mb-4">
        <button
          onClick={handleNewSession}
          className="w-full bg-primary text-white py-3 rounded-lg flex items-center justify-center gap-2 hover:bg-opacity-90 transition-colors"
        >
          <span>🐾</span>
          <span className="font-medium">新建对话</span>
        </button>
      </div>

      {/* 搜索框 */}
      <div className="px-6 mb-4">
        <div className="relative">
          <input
            type="text"
            placeholder="搜索对话..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full px-4 py-2 pl-10 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
          />
          <svg
            className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        </div>
      </div>

      {/* 会话列表 */}
      <div className="flex-1 overflow-y-auto px-6 space-y-4">
        {filteredSessions.length === 0 ? (
          <div className="text-center text-gray-400 mt-8">
            {searchQuery ? '没有找到匹配的对话' : '暂无对话'}
          </div>
        ) : (
          filteredSessions.map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              isActive={currentSession?.id === session.id}
              onClick={() => setCurrentSession(session)}
              onRename={(newName) => handleRenameSession(session.id, newName)}
              onDelete={() => handleDeleteSession(session.id)}
            />
          ))
        )}
      </div>

      {/* 工作区选择对话框 */}
      {showWorkspaceDialog && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-[500px] shadow-xl max-h-[80vh] overflow-y-auto">
            <h3 className="text-lg font-bold mb-4">选择工作区</h3>

            {!showCreateWorkspace ? (
              <>
                <div className="mb-4 max-h-[300px] overflow-y-auto space-y-2">
                  <label className="flex items-center p-3 border border-gray-300 rounded-lg hover:bg-gray-50 cursor-pointer">
                    <input
                      type="radio"
                      name="workspace"
                      value=""
                      checked={selectedWorkspaceId === ''}
                      onChange={(e) => setSelectedWorkspaceId(e.target.value)}
                      className="mr-3"
                    />
                    <div>
                      <div className="font-medium">无工作区</div>
                      <div className="text-xs text-gray-500">在当前目录下工作</div>
                    </div>
                  </label>
                  {workspaces.map((workspace) => (
                    <label
                      key={workspace.id}
                      className="flex items-center p-3 border border-gray-300 rounded-lg hover:bg-gray-50 cursor-pointer"
                    >
                      <input
                        type="radio"
                        name="workspace"
                        value={workspace.id}
                        checked={selectedWorkspaceId === workspace.id}
                        onChange={(e) => setSelectedWorkspaceId(e.target.value)}
                        className="mr-3"
                      />
                      <div className="flex-1 min-w-0">
                        <div className="font-medium truncate">{workspace.path}</div>
                        <div className="text-xs text-gray-500">
                          {workspace.type === 'self' ? '本项目' : '外部项目'}
                        </div>
                      </div>
                    </label>
                  ))}
                </div>

                <button
                  type="button"
                  onClick={() => setShowCreateWorkspace(true)}
                  className="w-full mb-4 px-4 py-2 border-2 border-dashed border-gray-300 text-gray-600 rounded-lg hover:border-primary hover:text-primary transition-colors"
                >
                  + 创建新工作区
                </button>

                <div className="flex justify-end gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setShowWorkspaceDialog(false);
                      setSelectedWorkspaceId('');
                    }}
                    className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded-lg transition-colors"
                  >
                    取消
                  </button>
                  <button
                    type="button"
                    onClick={handleWorkspaceDialogSubmit}
                    className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-opacity-90 transition-colors"
                  >
                    创建对话
                  </button>
                </div>
              </>
            ) : (
              <>
                <div className="mb-4">
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    工作区路径
                  </label>
                  <input
                    type="text"
                    value={newWorkspacePath}
                    onChange={(e) => setNewWorkspacePath(e.target.value)}
                    placeholder="/path/to/your/project"
                    className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
                  />
                  <p className="text-xs text-gray-500 mt-1">请输入绝对路径</p>
                </div>

                <div className="mb-4">
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    工作区类型
                  </label>
                  <div className="space-y-2">
                    <label className="flex items-center p-3 border border-gray-300 rounded-lg hover:bg-gray-50 cursor-pointer">
                      <input
                        type="radio"
                        name="workspaceType"
                        value="external"
                        checked={newWorkspaceType === 'external'}
                        onChange={(e) => setNewWorkspaceType(e.target.value as 'external')}
                        className="mr-3"
                      />
                      <div>
                        <div className="font-medium">外部项目</div>
                        <div className="text-xs text-gray-500">其他项目的工作区</div>
                      </div>
                    </label>
                    <label className="flex items-center p-3 border border-gray-300 rounded-lg hover:bg-gray-50 cursor-pointer">
                      <input
                        type="radio"
                        name="workspaceType"
                        value="self"
                        checked={newWorkspaceType === 'self'}
                        onChange={(e) => setNewWorkspaceType(e.target.value as 'self')}
                        className="mr-3"
                      />
                      <div>
                        <div className="font-medium">本项目</div>
                        <div className="text-xs text-gray-500">猫猫咖啡屋项目本身</div>
                      </div>
                    </label>
                  </div>
                </div>

                <div className="flex justify-end gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setShowCreateWorkspace(false);
                      setNewWorkspacePath('');
                    }}
                    className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded-lg transition-colors"
                  >
                    返回
                  </button>
                  <button
                    type="button"
                    onClick={handleCreateWorkspace}
                    className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-opacity-90 transition-colors"
                  >
                    创建工作区
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
