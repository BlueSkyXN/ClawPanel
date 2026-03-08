import { useState, useEffect, useCallback } from 'react';
import { useOutletContext } from 'react-router-dom';
import {
  Puzzle, Download, Trash2, RefreshCw, Settings2, Power, PowerOff,
  Search, Filter, ChevronRight, ExternalLink, AlertCircle, Check,
  FileText, Package, Tag, User, Globe, ArrowUpCircle, Terminal,
  Loader2, X,
} from 'lucide-react';
import { api } from '../lib/api';

interface PluginMeta {
  id: string;
  name: string;
  version: string;
  author: string;
  description: string;
  homepage?: string;
  repository?: string;
  category?: string;
  tags?: string[];
  icon?: string;
  configSchema?: any;
}

interface InstalledPlugin extends PluginMeta {
  enabled: boolean;
  installedAt: string;
  updatedAt?: string;
  source: string;
  dir: string;
  config?: Record<string, any>;
}

interface RegistryPlugin extends PluginMeta {
  downloads?: number;
  stars?: number;
  downloadUrl?: string;
  gitUrl?: string;
  screenshot?: string;
  readme?: string;
}

const CATEGORIES = [
  { key: 'all', label: '全部', labelEn: 'All' },
  { key: 'basic', label: '基础功能', labelEn: 'Basic' },
  { key: 'ai', label: 'AI 增强', labelEn: 'AI' },
  { key: 'message', label: '消息处理', labelEn: 'Message' },
  { key: 'fun', label: '娱乐互动', labelEn: 'Fun' },
  { key: 'tool', label: '工具', labelEn: 'Tool' },
];

export default function Plugins() {
  const { uiMode } = (useOutletContext() as { uiMode?: 'modern' }) || {};
  const modern = uiMode === 'modern';
  const [installed, setInstalled] = useState<InstalledPlugin[]>([]);
  const [registry, setRegistry] = useState<RegistryPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('all');
  const [tab, setTab] = useState<'market' | 'installed'>('market');
  const [selectedPlugin, setSelectedPlugin] = useState<string | null>(null);
  const [configPlugin, setConfigPlugin] = useState<string | null>(null);
  const [configData, setConfigData] = useState<Record<string, any>>({});
  const [configSchema, setConfigSchema] = useState<any>(null);
  const [logsPlugin, setLogsPlugin] = useState<string | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [installing, setInstalling] = useState<string | null>(null);
  const [customUrl, setCustomUrl] = useState('');
  const [showCustomInstall, setShowCustomInstall] = useState(false);
  const [actionMsg, setActionMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const loadPlugins = useCallback(async () => {
    try {
      setLoading(true);
      const r = await api.getPluginList();
      if (r.ok) {
        setInstalled(r.installed || []);
        setRegistry(r.registry || []);
      }
    } catch (e) {
      console.error('Failed to load plugins', e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadPlugins(); }, [loadPlugins]);

  const showMsg = (type: 'success' | 'error', text: string) => {
    setActionMsg({ type, text });
    setTimeout(() => setActionMsg(null), 3000);
  };

  const handleRefreshRegistry = async () => {
    setRefreshing(true);
    try {
      const r = await api.refreshPluginRegistry();
      if (r.ok) {
        setRegistry(r.registry?.plugins || []);
        showMsg('success', '插件仓库已刷新');
      } else {
        showMsg('error', r.error || '刷新失败');
      }
    } catch { showMsg('error', '刷新失败'); }
    setRefreshing(false);
  };

  const handleInstall = async (pluginId: string, source?: string) => {
    setInstalling(pluginId);
    try {
      const r = await api.installPlugin(pluginId, source);
      if (r.ok) {
        showMsg('success', r.message || `${pluginId} 安装任务已创建，请在消息中心查看进度`);
        setTimeout(() => { loadPlugins(); }, 5000);
      } else {
        showMsg('error', r.error || '安装失败');
      }
    } catch { showMsg('error', '安装失败'); }
    setInstalling(null);
  };

  const handleUninstall = async (pluginId: string) => {
    const cleanupConfig = window.confirm(`是否在卸载插件 ${pluginId} 时一并清理对应通道配置？\n\n选择“确定” = 卸载并清理配置\n选择“取消” = 仅卸载插件，保留配置`);
    const confirmed = cleanupConfig || window.confirm(`确认仅卸载插件 ${pluginId} 并保留配置？`);
    if (!confirmed) return;
    try {
      const r = await api.uninstallPlugin(pluginId, cleanupConfig);
      if (r.ok) {
        showMsg('success', r.message || `${pluginId} 卸载任务已创建，请在消息中心查看进度`);
        setTimeout(() => { loadPlugins(); }, 5000);
      } else {
        showMsg('error', r.error || '卸载失败');
      }
    } catch { showMsg('error', '卸载失败'); }
  };

  const handleToggle = async (pluginId: string, enabled: boolean) => {
    try {
      const r = await api.togglePlugin(pluginId, enabled);
      if (r.ok) {
        setInstalled(prev => prev.map(p => p.id === pluginId ? { ...p, enabled } : p));
        showMsg('success', enabled ? '插件已启用' : '插件已禁用');
      } else {
        showMsg('error', r.error || '操作失败');
      }
    } catch { showMsg('error', '操作失败'); }
  };

  const handleUpdate = async (pluginId: string) => {
    setInstalling(pluginId);
    try {
      const r = await api.updatePluginVersion(pluginId);
      if (r.ok) {
        showMsg('success', `${pluginId} 更新成功`);
        loadPlugins();
      } else {
        showMsg('error', r.error || '更新失败');
      }
    } catch { showMsg('error', '更新失败'); }
    setInstalling(null);
  };

  const openConfig = async (pluginId: string) => {
    try {
      const r = await api.getPluginConfig(pluginId);
      if (r.ok) {
        setConfigData(r.config || {});
        setConfigSchema(r.schema || null);
        setConfigPlugin(pluginId);
      }
    } catch {}
  };

  const saveConfig = async () => {
    if (!configPlugin) return;
    try {
      const r = await api.updatePluginConfig(configPlugin, configData);
      if (r.ok) {
        showMsg('success', '配置已保存');
        setConfigPlugin(null);
      } else {
        showMsg('error', r.error || '保存失败');
      }
    } catch { showMsg('error', '保存失败'); }
  };

  const openLogs = async (pluginId: string) => {
    try {
      const r = await api.getPluginLogs(pluginId);
      if (r.ok) {
        setLogs(r.logs || []);
        setLogsPlugin(pluginId);
      }
    } catch {}
  };

  const installedIds = new Set(installed.map(p => p.id));

  const filteredRegistry = registry.filter(p => {
    if (category !== 'all' && p.category !== category) return false;
    if (search) {
      const q = search.toLowerCase();
      return (
        p.name.toLowerCase().includes(q) ||
        p.id.toLowerCase().includes(q) ||
        (p.description || '').toLowerCase().includes(q) ||
        (p.author || '').toLowerCase().includes(q)
      );
    }
    return true;
  });

  const filteredInstalled = installed.filter(p => {
    if (search) {
      const q = search.toLowerCase();
      return p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q);
    }
    return true;
  });

  return (
    <div className={`space-y-6 ${modern ? 'page-modern' : ''}`}>
      {/* Header */}
      <div className={`${modern ? 'page-modern-header' : 'flex items-center justify-between'}`}>
        <div className="flex items-center gap-3">
          <div className={`${modern ? 'w-10 h-10 rounded-xl border border-blue-100/80 dark:border-blue-800/40 bg-[linear-gradient(135deg,rgba(37,99,235,0.16),rgba(14,165,233,0.1))] dark:bg-[linear-gradient(135deg,rgba(37,99,235,0.22),rgba(14,165,233,0.14))] flex items-center justify-center shadow-sm' : 'w-10 h-10 rounded-xl bg-gradient-to-br from-violet-500 to-purple-600 flex items-center justify-center shadow-lg shadow-violet-200 dark:shadow-none'}`}>
            <Puzzle className={`${modern ? 'text-blue-600 dark:text-blue-300' : 'text-white'}`} size={20} />
          </div>
          <div>
            <h1 className={`${modern ? 'page-modern-title text-xl' : 'text-xl font-bold text-gray-900 dark:text-white'}`}>插件中心</h1>
            <p className={`${modern ? 'page-modern-subtitle text-xs mt-0.5' : 'text-xs text-gray-500 mt-0.5'}`}>
              已安装 {installed.length} 个插件 · 仓库共 {registry.length} 个可用
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleRefreshRegistry}
            disabled={refreshing}
            className={`${modern ? 'page-modern-action text-violet-600 dark:text-violet-300' : 'flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg bg-violet-50 text-violet-600 hover:bg-violet-100 dark:bg-violet-900/30 dark:text-violet-300 dark:hover:bg-violet-900/50 transition-colors disabled:opacity-50'}`}
          >
            <RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />
            刷新仓库
          </button>
          <button
            onClick={() => setShowCustomInstall(true)}
            className={`${modern ? 'page-modern-action' : 'flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700 transition-colors'}`}
          >
            <Download size={14} />
            自定义安装
          </button>
        </div>
      </div>

      {/* Toast */}
      {actionMsg && (
        <div className={`fixed right-4 top-20 sm:top-6 z-[160] px-4 py-3 rounded-xl shadow-lg text-sm font-medium flex items-center gap-2 animate-in slide-in-from-right ${
          actionMsg.type === 'success'
            ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300'
            : 'bg-red-50 text-red-700 dark:bg-red-900/50 dark:text-red-300'
        }`}>
          {actionMsg.type === 'success' ? <Check size={16} /> : <AlertCircle size={16} />}
          {actionMsg.text}
        </div>
      )}

      {/* Tabs + Search */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3">
        <div className={`${modern ? 'inline-flex flex-wrap rounded-xl p-1 border border-blue-100/70 bg-[linear-gradient(145deg,rgba(255,255,255,0.78),rgba(239,246,255,0.62))] dark:bg-[linear-gradient(145deg,rgba(10,20,36,0.82),rgba(30,64,175,0.1))] dark:border-blue-800/20 shadow-sm backdrop-blur-xl' : 'flex bg-gray-100 dark:bg-gray-800 rounded-lg p-0.5'}`}>
          <button
            onClick={() => setTab('market')}
            className={`${modern ? 'px-4 py-2 text-xs font-medium rounded-lg transition-all border' : 'px-4 py-2 text-xs font-medium rounded-md transition-all'} ${
              tab === 'market'
                ? (modern ? 'border-blue-100/80 bg-blue-50/85 dark:bg-blue-900/20 dark:border-blue-800/40 text-blue-700 dark:text-blue-300 shadow-sm' : 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm')
                : (modern ? 'border-transparent text-gray-500 hover:bg-white/70 dark:hover:bg-slate-800/70 hover:text-gray-700 dark:hover:text-gray-300' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300')
            }`}
          >
            <Globe size={14} className="inline mr-1.5" />
            插件市场
          </button>
          <button
            onClick={() => setTab('installed')}
            className={`${modern ? 'px-4 py-2 text-xs font-medium rounded-lg transition-all border' : 'px-4 py-2 text-xs font-medium rounded-md transition-all'} ${
              tab === 'installed'
                ? (modern ? 'border-blue-100/80 bg-blue-50/85 dark:bg-blue-900/20 dark:border-blue-800/40 text-blue-700 dark:text-blue-300 shadow-sm' : 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm')
                : (modern ? 'border-transparent text-gray-500 hover:bg-white/70 dark:hover:bg-slate-800/70 hover:text-gray-700 dark:hover:text-gray-300' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300')
            }`}
          >
            <Package size={14} className="inline mr-1.5" />
            已安装 ({installed.length})
          </button>
        </div>

        <div className="flex-1 flex items-center gap-2">
          <div className="relative flex-1 max-w-sm">
            <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="搜索插件..."
              className="w-full pl-9 pr-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none"
            />
          </div>

          {tab === 'market' && (
            <div className="flex items-center gap-1 flex-wrap">
              {CATEGORIES.map(cat => (
                <button
                  key={cat.key}
                  onClick={() => setCategory(cat.key)}
                    className={`${modern ? 'px-2.5 py-1.5 text-[11px] font-medium rounded-lg transition-all border' : 'px-2.5 py-1.5 text-[11px] font-medium rounded-md transition-all'} ${
                      category === cat.key
                        ? (modern ? 'bg-blue-50/85 border-blue-100/80 text-blue-700 dark:bg-blue-900/20 dark:border-blue-800/40 dark:text-blue-300' : 'bg-violet-100 text-violet-700 dark:bg-violet-900/40 dark:text-violet-300')
                        : (modern ? 'border-transparent text-gray-500 hover:bg-white/70 dark:hover:bg-slate-800/70' : 'text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800')
                    }`}
                  >
                  {cat.label}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Loading */}
      {loading && (
        <div className="flex items-center justify-center py-12 text-gray-400">
          <Loader2 size={24} className="animate-spin mr-2" />
          加载中...
        </div>
      )}

      {/* Market Tab */}
      {!loading && tab === 'market' && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredRegistry.length === 0 ? (
            <div className="col-span-full text-center py-12 text-gray-400">
              <Package size={48} className="mx-auto mb-3 opacity-50" />
              <p className="text-sm">{search ? '未找到匹配的插件' : '插件仓库为空，点击「刷新仓库」获取最新列表'}</p>
            </div>
          ) : filteredRegistry.map(p => {
            const isInstalled = installedIds.has(p.id);
            return (
              <div key={p.id} className={`${modern ? 'relative overflow-hidden rounded-[24px] border border-white/65 dark:border-slate-700/50 bg-[linear-gradient(145deg,rgba(255,255,255,0.84),rgba(239,246,255,0.62))] dark:bg-[linear-gradient(145deg,rgba(15,23,42,0.88),rgba(30,64,175,0.10))] p-4 hover:shadow-md transition-all shadow-[0_18px_40px_rgba(15,23,42,0.06)] backdrop-blur-xl' : 'bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-4 hover:shadow-md transition-shadow'}`}>
                {modern && <div className="pointer-events-none absolute inset-x-5 top-0 h-px bg-gradient-to-r from-transparent via-white/90 to-transparent dark:via-slate-200/20" />}
                <div className="flex items-start gap-3">
                  <div className={`${modern ? 'w-10 h-10 rounded-xl border border-blue-100/80 dark:border-blue-800/30 bg-[linear-gradient(135deg,rgba(37,99,235,0.12),rgba(14,165,233,0.08))] dark:bg-[linear-gradient(135deg,rgba(37,99,235,0.2),rgba(14,165,233,0.12))] flex items-center justify-center text-blue-600 dark:text-blue-300 shrink-0 shadow-sm' : 'w-10 h-10 rounded-lg bg-gradient-to-br from-blue-100 to-violet-100 dark:from-blue-900/30 dark:to-violet-900/30 flex items-center justify-center text-violet-600 dark:text-violet-400 shrink-0'}`}>
                    <Puzzle size={20} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="font-semibold text-sm text-gray-900 dark:text-white truncate">{p.name}</h3>
                      <span className="text-[10px] px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 text-gray-500 rounded font-mono">v{p.version}</span>
                    </div>
                    <p className="text-xs text-gray-500 mt-1 line-clamp-2">{p.description || '暂无描述'}</p>
                    <div className="flex items-center gap-3 mt-2 text-[11px] text-gray-400">
                      {p.author && <span className="flex items-center gap-1"><User size={11} />{p.author}</span>}
                      {p.category && <span className="flex items-center gap-1"><Tag size={11} />{p.category}</span>}
                      {p.downloads != null && <span>{p.downloads} 下载</span>}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2 mt-3 pt-3 border-t border-gray-100 dark:border-gray-800">
                  {isInstalled ? (
                    <span className="flex items-center gap-1 text-xs text-emerald-600 dark:text-emerald-400 font-medium">
                      <Check size={14} /> 已安装
                    </span>
                  ) : (
                    <button
                      onClick={() => handleInstall(p.id)}
                      disabled={installing === p.id}
                      className={`${modern ? 'page-modern-accent px-3 py-1.5 text-xs' : 'flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-violet-600 text-white hover:bg-violet-700 transition-colors disabled:opacity-50'}`}
                    >
                      {installing === p.id ? <Loader2 size={13} className="animate-spin" /> : <Download size={13} />}
                      {installing === p.id ? '安装中...' : '安装'}
                    </button>
                  )}
                  {p.homepage && (
                    <a href={p.homepage} target="_blank" rel="noopener" className={`${modern ? 'page-modern-action ml-auto px-2.5 py-1.5 text-xs' : 'flex items-center gap-1 text-xs text-gray-400 hover:text-violet-500 transition-colors ml-auto'}`}>
                      <ExternalLink size={13} /> 主页
                    </a>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Installed Tab */}
      {!loading && tab === 'installed' && (
        <div className="space-y-3">
          {filteredInstalled.length === 0 ? (
            <div className="text-center py-12 text-gray-400">
              <Package size={48} className="mx-auto mb-3 opacity-50" />
              <p className="text-sm">{search ? '未找到匹配的已安装插件' : '暂无已安装的插件'}</p>
              <button onClick={() => setTab('market')} className="mt-3 text-xs text-violet-500 hover:underline">
                前往插件市场
              </button>
            </div>
          ) : filteredInstalled.map(p => (
            <div key={p.id} className={`${modern ? 'relative overflow-hidden rounded-[24px] border border-white/65 dark:border-slate-700/50 bg-[linear-gradient(145deg,rgba(255,255,255,0.84),rgba(239,246,255,0.62))] dark:bg-[linear-gradient(145deg,rgba(15,23,42,0.88),rgba(30,64,175,0.10))] p-4 shadow-[0_18px_40px_rgba(15,23,42,0.06)] backdrop-blur-xl' : 'bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-4'}`}>
              {modern && <div className="pointer-events-none absolute inset-x-5 top-0 h-px bg-gradient-to-r from-transparent via-white/90 to-transparent dark:via-slate-200/20" />}
              <div className="flex items-center gap-4">
                <div className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${
                  p.enabled
                    ? 'bg-gradient-to-br from-emerald-100 to-green-100 dark:from-emerald-900/30 dark:to-green-900/30 text-emerald-600 dark:text-emerald-400'
                    : 'bg-gray-100 dark:bg-gray-800 text-gray-400'
                }`}>
                  <Puzzle size={20} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold text-sm text-gray-900 dark:text-white">{p.name}</h3>
                    <span className="text-[10px] px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 text-gray-500 rounded font-mono">v{p.version}</span>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${
                      p.enabled
                        ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400'
                        : 'bg-gray-100 text-gray-500 dark:bg-gray-800'
                    }`}>
                      {p.enabled ? '已启用' : '已禁用'}
                    </span>
                  </div>
                  <p className="text-xs text-gray-500 mt-0.5">{p.description || '暂无描述'}</p>
                  <div className="flex items-center gap-3 mt-1 text-[11px] text-gray-400">
                    {p.author && <span><User size={11} className="inline mr-0.5" />{p.author}</span>}
                    <span>来源: {p.source}</span>
                    <span>安装于 {new Date(p.installedAt).toLocaleDateString()}</span>
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  <button
                    onClick={() => handleToggle(p.id, !p.enabled)}
                    className={`${modern ? (p.enabled ? 'page-modern-success p-2' : 'page-modern-action p-2') : `p-2 rounded-lg transition-colors ${
                      p.enabled
                        ? 'text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-900/20'
                        : 'text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                    }`}`}
                    title={p.enabled ? '禁用' : '启用'}
                  >
                    {p.enabled ? <Power size={16} /> : <PowerOff size={16} />}
                  </button>
                  <button
                    onClick={() => openConfig(p.id)}
                    className={`${modern ? 'page-modern-action p-2' : 'p-2 rounded-lg text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300 transition-colors'}`}
                    title="配置"
                  >
                    <Settings2 size={16} />
                  </button>
                  <button
                    onClick={() => openLogs(p.id)}
                    className={`${modern ? 'page-modern-action p-2' : 'p-2 rounded-lg text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300 transition-colors'}`}
                    title="日志"
                  >
                    <Terminal size={16} />
                  </button>
                  <button
                    onClick={() => handleUpdate(p.id)}
                    disabled={installing === p.id}
                    className={`${modern ? 'page-modern-accent p-2' : 'p-2 rounded-lg text-blue-500 hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors disabled:opacity-50'}`}
                    title="更新"
                  >
                    {installing === p.id ? <Loader2 size={16} className="animate-spin" /> : <ArrowUpCircle size={16} />}
                  </button>
                  <button
                    onClick={() => handleUninstall(p.id)}
                    className={`${modern ? 'page-modern-danger p-2' : 'p-2 rounded-lg text-red-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 transition-colors'}`}
                    title="卸载"
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Custom Install Modal */}
      {showCustomInstall && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={() => setShowCustomInstall(false)}>
          <div className={`${modern ? 'bg-[linear-gradient(145deg,rgba(255,255,255,0.92),rgba(239,246,255,0.72))] dark:bg-[linear-gradient(145deg,rgba(12,24,42,0.92),rgba(30,64,175,0.14))] rounded-2xl shadow-xl w-full max-w-md p-6 space-y-4 border border-blue-100/70 dark:border-blue-800/20 backdrop-blur-xl' : 'bg-white dark:bg-gray-900 rounded-2xl shadow-xl w-full max-w-md p-6 space-y-4'}`} onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <h3 className="font-semibold text-gray-900 dark:text-white">自定义安装插件</h3>
              <button onClick={() => setShowCustomInstall(false)} className={`${modern ? 'page-modern-action p-1.5' : 'text-gray-400 hover:text-gray-600'}`}><X size={18} /></button>
            </div>
            <p className="text-xs text-gray-500">输入插件 Git 仓库 URL 或下载链接进行安装</p>
            <input
              value={customUrl}
              onChange={e => setCustomUrl(e.target.value)}
              placeholder="https://github.com/user/plugin.git 或 .zip URL"
              className="w-full px-3 py-2.5 text-sm rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none"
            />
            <div className="flex justify-end gap-2">
              <button onClick={() => setShowCustomInstall(false)} className={`${modern ? 'page-modern-action px-4 py-2 text-xs' : 'px-4 py-2 text-xs font-medium rounded-lg text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800'}`}>取消</button>
              <button
                onClick={() => {
                  if (!customUrl.trim()) return;
                  const pluginId = customUrl.split('/').pop()?.replace(/\.git$/, '').replace(/\.zip$/, '') || 'custom-plugin';
                  handleInstall(pluginId, customUrl);
                  setShowCustomInstall(false);
                  setCustomUrl('');
                }}
                disabled={!customUrl.trim()}
                className={`${modern ? 'page-modern-accent px-4 py-2 text-xs' : 'px-4 py-2 text-xs font-medium rounded-lg bg-violet-600 text-white hover:bg-violet-700 disabled:opacity-50'}`}
              >
                安装
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Config Modal */}
      {configPlugin && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={() => setConfigPlugin(null)}>
          <div className={`${modern ? 'bg-[linear-gradient(145deg,rgba(255,255,255,0.92),rgba(239,246,255,0.72))] dark:bg-[linear-gradient(145deg,rgba(12,24,42,0.92),rgba(30,64,175,0.14))] rounded-2xl shadow-xl w-full max-w-lg p-6 space-y-4 max-h-[80vh] overflow-y-auto border border-blue-100/70 dark:border-blue-800/20 backdrop-blur-xl' : 'bg-white dark:bg-gray-900 rounded-2xl shadow-xl w-full max-w-lg p-6 space-y-4 max-h-[80vh] overflow-y-auto'}`} onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <h3 className="font-semibold text-gray-900 dark:text-white">插件配置 - {configPlugin}</h3>
              <button onClick={() => setConfigPlugin(null)} className={`${modern ? 'page-modern-action p-1.5' : 'text-gray-400 hover:text-gray-600'}`}><X size={18} /></button>
            </div>

            {configSchema ? (
              <DynamicConfigForm schema={configSchema} data={configData} onChange={setConfigData} />
            ) : (
              <div className="space-y-3">
                <p className="text-xs text-gray-500">此插件未提供 JSON Schema，您可以直接编辑 JSON 配置：</p>
                <textarea
                  value={JSON.stringify(configData, null, 2)}
                  onChange={e => { try { setConfigData(JSON.parse(e.target.value)); } catch {} }}
                  rows={12}
                  className="w-full px-3 py-2 text-xs font-mono rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800 focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none"
                />
              </div>
            )}

            <div className="flex justify-end gap-2 pt-2">
              <button onClick={() => setConfigPlugin(null)} className={`${modern ? 'page-modern-action px-4 py-2 text-xs' : 'px-4 py-2 text-xs font-medium rounded-lg text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800'}`}>取消</button>
              <button onClick={saveConfig} className={`${modern ? 'page-modern-accent px-4 py-2 text-xs' : 'px-4 py-2 text-xs font-medium rounded-lg bg-violet-600 text-white hover:bg-violet-700'}`}>保存配置</button>
            </div>
          </div>
        </div>
      )}

      {/* Logs Modal */}
      {logsPlugin && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={() => setLogsPlugin(null)}>
          <div className={`${modern ? 'bg-[linear-gradient(145deg,rgba(255,255,255,0.92),rgba(239,246,255,0.72))] dark:bg-[linear-gradient(145deg,rgba(12,24,42,0.92),rgba(30,64,175,0.14))] rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4 max-h-[80vh] overflow-hidden flex flex-col border border-blue-100/70 dark:border-blue-800/20 backdrop-blur-xl' : 'bg-white dark:bg-gray-900 rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4 max-h-[80vh] overflow-hidden flex flex-col'}`} onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <h3 className="font-semibold text-gray-900 dark:text-white flex items-center gap-2">
                <Terminal size={16} /> 插件日志 - {logsPlugin}
              </h3>
              <button onClick={() => setLogsPlugin(null)} className={`${modern ? 'page-modern-action p-1.5' : 'text-gray-400 hover:text-gray-600'}`}><X size={18} /></button>
            </div>
            <div className="flex-1 overflow-y-auto bg-gray-950 rounded-lg p-4 font-mono text-xs text-green-400 min-h-[200px]">
              {logs.length === 0 ? (
                <p className="text-gray-500">暂无日志</p>
              ) : logs.map((line, i) => (
                <div key={i} className="whitespace-pre-wrap break-all">{line}</div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// Dynamic config form from JSON Schema
function DynamicConfigForm({ schema, data, onChange }: { schema: any; data: Record<string, any>; onChange: (d: Record<string, any>) => void }) {
  let parsed: any = {};
  try {
    parsed = typeof schema === 'string' ? JSON.parse(schema) : schema;
  } catch {
    return <p className="text-xs text-gray-500">无法解析配置 Schema</p>;
  }

  const properties = parsed.properties || {};
  const required = parsed.required || [];

  const setField = (key: string, value: any) => {
    onChange({ ...data, [key]: value });
  };

  return (
    <div className="space-y-4">
      {Object.entries(properties).map(([key, prop]: [string, any]) => {
        const val = data[key];
        const label = prop.title || prop.description || key;
        const isRequired = required.includes(key);

        return (
          <div key={key}>
            <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
              {label} {isRequired && <span className="text-red-500">*</span>}
            </label>
            {prop.type === 'boolean' ? (
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={!!val}
                  onChange={e => setField(key, e.target.checked)}
                  className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                <span className="text-xs text-gray-600 dark:text-gray-400">{prop.description || ''}</span>
              </label>
            ) : prop.type === 'number' || prop.type === 'integer' ? (
              <input
                type="number"
                value={val ?? ''}
                onChange={e => setField(key, e.target.value ? Number(e.target.value) : undefined)}
                placeholder={prop.default?.toString()}
                className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 focus:ring-2 focus:ring-blue-500/20 outline-none"
              />
            ) : prop.enum ? (
              <select
                value={val ?? ''}
                onChange={e => setField(key, e.target.value)}
                className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 focus:ring-2 focus:ring-blue-500/20 outline-none"
              >
                <option value="">-- 选择 --</option>
                {prop.enum.map((v: string) => <option key={v} value={v}>{v}</option>)}
              </select>
            ) : (
              <input
                type="text"
                value={val ?? ''}
                onChange={e => setField(key, e.target.value)}
                placeholder={prop.default?.toString() || ''}
                className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 focus:ring-2 focus:ring-blue-500/20 outline-none"
              />
            )}
            {prop.description && prop.type !== 'boolean' && (
              <p className="text-[11px] text-gray-400 mt-0.5">{prop.description}</p>
            )}
          </div>
        );
      })}
    </div>
  );
}
