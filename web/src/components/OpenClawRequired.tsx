import { Brain, Download, Loader2, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { api } from '../lib/api';
import { ensureOpenClawInstallPrerequisites, getOpenClawInstallPrerequisiteStatus } from '../lib/openclawPrereq';
import { resolveOpenClawRuntime } from '../lib/openclawRuntime';

interface Props {
  openclawStatus?: any;
  processStatus?: any;
  children: React.ReactNode;
}

export default function OpenClawRequired({ openclawStatus, processStatus, children }: Props) {
  const { pathname } = useLocation();
  const dismissKey = `openclaw-required-dismissed:${pathname}`;
  const configured = !!openclawStatus?.configured;
  const isLiteEdition = openclawStatus?.edition === 'lite';
  const runtime = resolveOpenClawRuntime(openclawStatus, processStatus);
  const [installing, setInstalling] = useState(false);
  const [dismissed, setDismissed] = useState(() => sessionStorage.getItem(dismissKey) === '1');
  const [installBlocked, setInstallBlocked] = useState(false);
  const [installBlockedMessage, setInstallBlockedMessage] = useState('');
  const [nodeUrl, setNodeUrl] = useState('https://nodejs.org');
  const [gitUrl, setGitUrl] = useState('https://git-scm.com/downloads');

  useEffect(() => {
    if (configured) {
      const keysToClear: string[] = [];
      for (let i = 0; i < sessionStorage.length; i += 1) {
        const key = sessionStorage.key(i);
        if (key?.startsWith('openclaw-required-dismissed:')) keysToClear.push(key);
      }
      keysToClear.forEach(key => sessionStorage.removeItem(key));
      setDismissed(false);
    }
  }, [configured, dismissKey]);

  useEffect(() => {
    setDismissed(sessionStorage.getItem(dismissKey) === '1');
  }, [dismissKey]);

  useEffect(() => {
    if (isLiteEdition) {
      setInstallBlocked(true);
      setInstallBlockedMessage('Lite 版已内置 OpenClaw；若当前未就绪，请检查内置 runtime 是否完整或重新安装 Lite。');
      return;
    }
    let active = true;
    getOpenClawInstallPrerequisiteStatus().then(status => {
      if (!active) return;
      setInstallBlocked(status.requiresManualInstall);
      setInstallBlockedMessage(status.message || '');
      setNodeUrl(status.nodeUrl);
      setGitUrl(status.gitUrl);
    }).catch(() => {
      if (!active) return;
      setInstallBlocked(false);
      setInstallBlockedMessage('');
    });
    return () => { active = false; };
  }, [isLiteEdition]);

  if (configured && runtime.healthy) return <>{children}</>;

  if (configured && !runtime.healthy) {
    return (
      <div className="space-y-4">
        <div className={`rounded-2xl border px-4 py-4 flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between ${runtime.state === 'offline' ? 'border-red-200/80 dark:border-red-900/40 bg-red-50/90 dark:bg-red-950/20' : 'border-amber-200/80 dark:border-amber-900/40 bg-amber-50/90 dark:bg-amber-950/20'}`}>
          <div>
            <div className={`text-sm font-semibold ${runtime.state === 'offline' ? 'text-red-900 dark:text-red-100' : 'text-amber-900 dark:text-amber-100'}`}>{runtime.title}</div>
            <p className={`text-xs mt-1 leading-5 ${runtime.state === 'offline' ? 'text-red-700 dark:text-red-200/90' : 'text-amber-700 dark:text-amber-200/90'}`}>{runtime.message}</p>
          </div>
          <div className="flex flex-wrap gap-2 shrink-0">
            <button
              onClick={async () => {
                try {
                  const r = await api.restartGateway();
                  if (!r?.ok) window.alert(r?.error || '重启网关失败');
                } catch {
                  window.alert('重启网关失败');
                }
              }}
              className="px-4 py-2 text-xs font-medium rounded-xl border border-blue-200 text-blue-700 hover:bg-blue-50 transition-colors"
            >
              重启网关
            </button>
          </div>
        </div>
        <div className={`${runtime.state === 'offline' ? 'opacity-70' : ''}`}>
          {children}
        </div>
      </div>
    );
  }

  const handleInstall = async () => {
    setInstalling(true);
    try {
      if (isLiteEdition) return;
      const status = await ensureOpenClawInstallPrerequisites();
      if (status.requiresManualInstall) {
        setInstallBlocked(true);
        setInstallBlockedMessage(status.message || '请先手动安装 Node.js 与 Git');
        return;
      }
      await api.installSoftware('openclaw');
    } catch {}
    finally { setInstalling(false); }
  };

  const dismiss = () => {
    sessionStorage.setItem(dismissKey, '1');
    setDismissed(true);
  };

  const reopen = () => {
    sessionStorage.removeItem(dismissKey);
    setDismissed(false);
  };

  if (dismissed) {
    return (
      <div className="space-y-3">
        <div className="rounded-2xl border border-amber-200/80 dark:border-amber-900/50 bg-amber-50/90 dark:bg-amber-950/20 px-4 py-3 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <div className="text-sm font-semibold text-amber-900 dark:text-amber-200">OpenClaw 尚未安装或配置</div>
            <p className="text-xs text-amber-700 dark:text-amber-300 mt-1">
              当前页面仍可浏览，但部分实时数据和保存功能可能暂时不可用。
            </p>
            {installBlockedMessage && <p className="text-xs text-amber-700 dark:text-amber-300 mt-2">{installBlockedMessage}</p>}
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              onClick={handleInstall}
              disabled={installing || installBlocked}
              className="inline-flex items-center gap-2 px-4 py-2 text-xs font-medium rounded-xl bg-violet-600 text-white hover:bg-violet-700 disabled:opacity-50 transition-all"
            >
              {installing ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
              {installing ? '安装中...' : (isLiteEdition ? 'Lite 已内置 OpenClaw' : '安装 OpenClaw')}
            </button>
            {installBlocked && (
              <>
                 {!isLiteEdition && <button onClick={() => window.open(nodeUrl, '_blank', 'noopener,noreferrer')} className="px-4 py-2 text-xs font-medium rounded-xl border border-blue-200 text-blue-700 hover:bg-blue-50 transition-colors">下载 Node.js</button>}
                 {!isLiteEdition && <button onClick={() => window.open(gitUrl, '_blank', 'noopener,noreferrer')} className="px-4 py-2 text-xs font-medium rounded-xl border border-blue-200 text-blue-700 hover:bg-blue-50 transition-colors">下载 Git</button>}
              </>
            )}
            <button
              onClick={reopen}
              className="px-4 py-2 text-xs font-medium rounded-xl border border-amber-300/80 dark:border-amber-800 text-amber-700 dark:text-amber-300 hover:bg-amber-100/70 dark:hover:bg-amber-900/30 transition-colors"
            >
              重新显示提示
            </button>
          </div>
        </div>
        {children}
      </div>
    );
  }

  return (
    <div className="relative">
      {/* Greyed out content */}
      <div className="opacity-20 pointer-events-none select-none blur-[2px]">
        {children}
      </div>
      {/* Overlay */}
      <div className="absolute inset-0 flex items-center justify-center z-10">
        <div className="relative bg-white/95 dark:bg-gray-900/95 backdrop-blur-sm rounded-2xl shadow-2xl border border-gray-200 dark:border-gray-700 p-8 max-w-md text-center space-y-4">
          <button
            onClick={dismiss}
            className="absolute top-4 right-4 p-2 rounded-xl text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
            title="关闭提示并继续查看页面"
          >
            <X size={16} />
          </button>
          <div className="w-14 h-14 mx-auto rounded-2xl bg-amber-100 dark:bg-amber-900/30 flex items-center justify-center">
            <Brain size={28} className="text-amber-600 dark:text-amber-400" />
          </div>
          <div>
              <h3 className="text-lg font-bold text-gray-900 dark:text-white">{isLiteEdition ? 'Lite 内置 OpenClaw 未就绪' : '需要安装或配置 OpenClaw'}</h3>
              <p className="text-sm text-gray-500 mt-1">
                {isLiteEdition
                  ? 'Lite 版默认自带 OpenClaw。若当前仍不可用，请检查安装包是否完整，或重新安装 Lite。'
                  : '此功能依赖 OpenClaw AI 引擎。你可以先安装 / 配置，也可以先关闭提示继续调试页面结构。'}
              </p>
            {installBlockedMessage && <p className="text-xs text-amber-600 dark:text-amber-300 mt-3 leading-5">{installBlockedMessage}</p>}
          </div>
          <div className="flex flex-col sm:flex-row gap-3 justify-center">
            <button onClick={handleInstall} disabled={installing || installBlocked}
              className="page-modern-accent inline-flex items-center justify-center gap-2 px-6 py-3 text-sm disabled:opacity-50">
              {installing ? <Loader2 size={16} className="animate-spin" /> : <Download size={16} />}
              {installing ? '安装中...' : (isLiteEdition ? 'Lite 已内置 OpenClaw' : '一键安装 OpenClaw')}
            </button>
            {installBlocked && (
              <>
                {!isLiteEdition && <button onClick={() => window.open(nodeUrl, '_blank', 'noopener,noreferrer')} className="inline-flex items-center justify-center gap-2 px-6 py-3 text-sm font-medium rounded-xl border border-blue-200 text-blue-700 hover:bg-blue-50 transition-colors">下载 Node.js</button>}
                {!isLiteEdition && <button onClick={() => window.open(gitUrl, '_blank', 'noopener,noreferrer')} className="inline-flex items-center justify-center gap-2 px-6 py-3 text-sm font-medium rounded-xl border border-blue-200 text-blue-700 hover:bg-blue-50 transition-colors">下载 Git</button>}
              </>
            )}
            <button
              onClick={dismiss}
              className="inline-flex items-center justify-center gap-2 px-6 py-3 text-sm font-medium rounded-xl border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
            >
              关闭提示，继续查看页面
            </button>
          </div>
          <p className="text-[11px] text-gray-400">安装进度可在右上角铃铛中的消息中心实时查看</p>
        </div>
      </div>
    </div>
  );
}
