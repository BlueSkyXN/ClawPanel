import { useEffect, useMemo, useState } from 'react';
import { useOutletContext } from 'react-router-dom';
import {
  Bot,
  CheckCircle2,
  Download,
  Eye,
  ExternalLink,
  FileCode,
  Trash2,
  GitBranch,
  Loader2,
  Pause,
  Play,
  RefreshCw,
  Save,
  Sparkles,
  Square,
  Wand2,
  X,
  XCircle,
} from 'lucide-react';
import { api } from '../lib/api';
import MobileActionTray from '../components/MobileActionTray';

interface WorkflowSettings {
  enabled: boolean;
  providerId: string;
  modelId: string;
  approvalMode: number;
  progressMode: string;
  tone: string;
  autoCreateRuns: boolean;
  pushProgress: boolean;
  complexityGuard: string;
}

interface WorkflowTemplate {
  id: string;
  name: string;
  description: string;
  category: string;
  status: string;
  triggerMode: string;
  settings?: Record<string, any>;
  definition?: Record<string, any>;
  createdAt?: number;
  updatedAt?: number;
}

interface WorkflowStep {
  id: string;
  stepKey: string;
  title: string;
  stepType: string;
  status: string;
  needsApproval?: boolean;
  outputText?: string;
  errorText?: string;
}

interface WorkflowRun {
  id: string;
  shortId: string;
  templateId: string;
  name: string;
  status: string;
  conversationId?: string;
  channelId?: string;
  userId?: string;
  sourceMessage?: string;
  lastMessage?: string;
  context?: Record<string, any>;
  settings?: Record<string, any>;
  updatedAt?: number;
  createdAt?: number;
  steps?: WorkflowStep[];
}

interface WorkflowEvent {
  id: number;
  stepId?: string;
  eventType: string;
  message: string;
  createdAt?: number;
}

interface WorkflowArtifact {
  stepKey?: string;
  title?: string;
  artifactName?: string;
  fileName?: string;
  relativePath?: string;
  absolutePath?: string;
  isFinalOutput?: boolean;
  sendToUser?: boolean;
  updatedAt?: number;
  deliveryStatus?: string;
  deliveryChannel?: string;
  deliveryMode?: string;
  deliveryTarget?: string;
  deliveryUpdatedAt?: number;
  deliveryError?: string;
}

interface ArtifactPreviewState {
  path: string;
  title: string;
  content: string;
  isMarkdown?: boolean;
  truncated?: boolean;
}

interface ModelOption {
  value: string;
  label: string;
}

const DEFAULT_SETTINGS: WorkflowSettings = {
  enabled: false,
  providerId: '',
  modelId: '',
  approvalMode: 2,
  progressMode: 'detailed',
  tone: 'professional',
  autoCreateRuns: true,
  pushProgress: true,
  complexityGuard: 'balanced',
};

const DEFAULT_TEMPLATE_JSON = JSON.stringify(
  {
    nodes: [
      { id: 'collect', title: '收集输入', type: 'input', order: 1 },
      { id: 'plan', title: '拆解计划', type: 'ai_plan', order: 2, skill: 'none', outputFile: '01-拆解计划.md', artifactName: '拆解计划', outputFormat: 'markdown' },
      { id: 'execute', title: '执行任务', type: 'ai_task', order: 3, skill: 'none', outputFile: '02-执行结果.md', artifactName: '执行结果', outputFormat: 'markdown', isFinalOutput: true },
      { id: 'review', title: '等待确认', type: 'approval', order: 4, skill: 'none' },
      { id: 'end', title: '完成', type: 'end', order: 5 },
    ],
    edges: [],
  },
  null,
  2,
);

function extractModelOptions(payload: any): ModelOption[] {
  const providers = payload?.providers || payload?.models?.providers || {};
  const items: ModelOption[] = [];
  Object.entries(providers).forEach(([providerId, providerValue]) => {
    const provider = providerValue as any;
    const models = provider?.models || {};
    Object.entries(models).forEach(([modelId, modelValue]) => {
      const model = modelValue as any;
      const label = model?.name || model?.label || `${providerId}/${modelId}`;
      items.push({ value: `${providerId}/${modelId}`, label });
    });
  });
  return items;
}

function formatTime(value?: number) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function toneLabel(value: string) {
  if (value === 'operations') return '运营协作';
  if (value === 'friendly') return '友好助手';
  return '专业简洁';
}

function statusTone(status: string) {
  if (status === 'completed') return 'text-emerald-600 bg-emerald-50 dark:bg-emerald-900/20 dark:text-emerald-300';
  if (status === 'running') return 'text-blue-600 bg-blue-50 dark:bg-blue-900/20 dark:text-blue-300';
  if (status === 'paused') return 'text-amber-600 bg-amber-50 dark:bg-amber-900/20 dark:text-amber-300';
  if (status === 'waiting_for_user' || status === 'waiting_for_approval') return 'text-orange-600 bg-orange-50 dark:bg-orange-900/20 dark:text-orange-300';
  if (status === 'failed' || status === 'cancelled') return 'text-red-600 bg-red-50 dark:bg-red-900/20 dark:text-red-300';
  return 'text-slate-600 bg-slate-100 dark:bg-slate-800 dark:text-slate-300';
}

function statusLabel(status: string) {
  if (status === 'completed') return '已完成';
  if (status === 'running') return '执行中';
  if (status === 'paused') return '已暂停';
  if (status === 'waiting_for_user') return '等待用户';
  if (status === 'waiting_for_approval') return '等待审批';
  if (status === 'failed') return '失败';
  if (status === 'cancelled') return '已取消';
  if (status === 'pending') return '待执行';
  if (status === 'blocked') return '阻塞中';
  return status || '未知';
}

function channelLabel(value?: string) {
  if (value === 'qq') return 'QQ';
  if (value === 'wecom') return '企业微信';
  if (value === 'feishu') return '飞书';
  return value || '未知通道';
}

function isTakeoverRun(run?: WorkflowRun | null) {
  return Boolean(run?.context?.autoCreated);
}

function isTakeoverActive(run?: WorkflowRun | null) {
  if (!run || !isTakeoverRun(run)) return false;
  return run.status === 'running' || run.status === 'waiting_for_user' || run.status === 'waiting_for_approval' || run.status === 'paused';
}

function takeoverBadge(run?: WorkflowRun | null) {
  if (!run || !isTakeoverRun(run)) return null;
  if (isTakeoverActive(run)) return '原会话接管中';
  return '原会话已接管';
}

function originPushStatus(run?: WorkflowRun | null) {
  return String(run?.context?.originPushStatus || '').trim();
}

function originPushLabel(run?: WorkflowRun | null) {
  const status = originPushStatus(run);
  if (status === 'sent') return '原通道已回写';
  if (status === 'failed') return '原通道回写失败';
  return null;
}

function originPushTone(run?: WorkflowRun | null) {
  const status = originPushStatus(run);
  if (status === 'sent') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300';
  if (status === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300';
  return 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300';
}

function artifactFilesFromRun(run?: WorkflowRun | null): WorkflowArtifact[] {
  const raw = run?.context?.artifactFiles;
  if (!Array.isArray(raw)) return [];
  return raw as WorkflowArtifact[];
}

function finalArtifactFilesFromRun(run?: WorkflowRun | null): WorkflowArtifact[] {
  const raw = run?.context?.finalFiles;
  if (!Array.isArray(raw)) return [];
  return raw as WorkflowArtifact[];
}

function artifactDeliveryLabel(status?: string) {
  if (status === 'sent') return '已回传';
  if (status === 'partial') return '部分回传';
  if (status === 'failed') return '回传失败';
  if (status === 'skipped') return '未回传';
  return '待回传';
}

function artifactDeliveryTone(status?: string) {
  if (status === 'sent') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300';
  if (status === 'partial') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300';
  if (status === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300';
  return 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300';
}

function artifactDeliveryTargetLabel(run?: WorkflowRun | null) {
  const mode = String(run?.context?.artifactDeliveryMode || '').trim();
  const target = String(run?.context?.artifactDeliveryTarget || '').trim();
  if (!mode) return '-';
  if (mode === 'group') return `QQ 群聊 ${target || ''}`.trim();
  if (mode === 'private') return `QQ 私聊 ${target || ''}`.trim();
  return target || mode;
}

function artifactFilePreviewUrl(path?: string) {
  if (!path) return '';
  return api.workspacePreviewUrl(path);
}

function artifactFileDownloadUrl(path?: string) {
  if (!path) return '';
  return api.workspaceDownloadUrl(path);
}

function artifactFileExtension(path?: string, fileName?: string) {
  const source = String(fileName || path || '').toLowerCase();
  const match = source.match(/\.([a-z0-9]+)$/);
  return match?.[1] || '';
}

function artifactPreviewLabel(path?: string, fileName?: string) {
  const ext = artifactFileExtension(path, fileName);
  if (['md', 'markdown', 'txt', 'json', 'yaml', 'yml', 'csv', 'log'].includes(ext)) return '在线预览';
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) return '查看图片';
  if (['pdf'].includes(ext)) return '查看 PDF';
  return '打开文件';
}

function artifactTypeBadge(path?: string, fileName?: string) {
  const ext = artifactFileExtension(path, fileName);
  if (['md', 'markdown'].includes(ext)) return 'Markdown';
  if (['txt', 'log'].includes(ext)) return '文本';
  if (['json', 'yaml', 'yml'].includes(ext)) return '数据';
  if (['csv'].includes(ext)) return '表格';
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) return '图片';
  if (['pdf'].includes(ext)) return 'PDF';
  return ext ? ext.toUpperCase() : '文件';
}

function simpleMarkdown(md: string): string {
  let html = md
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/^######\s+(.+)$/gm, '<h6>$1</h6>')
    .replace(/^#####\s+(.+)$/gm, '<h5>$1</h5>')
    .replace(/^####\s+(.+)$/gm, '<h4>$1</h4>')
    .replace(/^###\s+(.+)$/gm, '<h3>$1</h3>')
    .replace(/^##\s+(.+)$/gm, '<h2>$1</h2>')
    .replace(/^#\s+(.+)$/gm, '<h1>$1</h1>')
    .replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code class="bg-gray-100 dark:bg-gray-800 px-1 py-0.5 rounded text-xs">$1</code>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_m: string, text: string, url: string) => {
      const trimmedUrl = url.trim().toLowerCase();
      if (trimmedUrl.startsWith('javascript:') || trimmedUrl.startsWith('data:') || trimmedUrl.startsWith('vbscript:')) {
        return text;
      }
      const safeUrl = url.replace(/"/g, '&quot;');
      const safeText = text.replace(/"/g, '&quot;');
      return `<a href="${safeUrl}" target="_blank" rel="noopener" class="text-indigo-600 hover:underline">${safeText}</a>`;
    })
    .replace(/^---+$/gm, '<hr class="my-4 border-gray-200 dark:border-gray-700" />')
    .replace(/^[-*]\s+(.+)$/gm, '<li>$1</li>')
    .replace(/^\d+\.\s+(.+)$/gm, '<li>$1</li>')
    .replace(/^&gt;\s+(.+)$/gm, '<blockquote class="border-l-4 border-gray-300 dark:border-gray-600 pl-3 text-gray-600 dark:text-gray-400 italic">$1</blockquote>');
  html = html.replace(/((?:<li>.*<\/li>\n?)+)/g, '<ul class="list-disc pl-5 space-y-1">$1</ul>');
  html = html.split('\n').map(line => {
    const trimmed = line.trim();
    if (!trimmed) return '';
    if (/^<(h[1-6]|ul|ol|li|blockquote|hr|pre|code|div|table)/.test(trimmed)) return line;
    return `<p>${line}</p>`;
  }).join('\n');
  return html;
}

function supportsInlinePreview(path?: string, fileName?: string) {
  const ext = artifactFileExtension(path, fileName);
  return ['md', 'markdown', 'txt', 'json', 'yaml', 'yml', 'csv', 'log'].includes(ext);
}

function isMarkdownArtifact(path?: string, fileName?: string) {
  const ext = artifactFileExtension(path, fileName);
  return ['md', 'markdown'].includes(ext);
}

export default function Workflows() {
  const { uiMode } = (useOutletContext() as { uiMode?: 'modern' }) || {};
  const modern = uiMode === 'modern';

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [settings, setSettings] = useState<WorkflowSettings>(DEFAULT_SETTINGS);
  const [modelOptions, setModelOptions] = useState<ModelOption[]>([]);
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState('');
  const [templateDraft, setTemplateDraft] = useState<WorkflowTemplate>({
    id: '',
    name: '',
    description: '',
    category: '',
    status: 'ready',
    triggerMode: 'manual',
    settings: {},
    definition: { nodes: [], edges: [] },
  });
  const [templateDefinitionText, setTemplateDefinitionText] = useState(DEFAULT_TEMPLATE_JSON);
  const [generatePrompt, setGeneratePrompt] = useState('');
  const [generating, setGenerating] = useState(false);
  const [runFilter, setRunFilter] = useState('');
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [selectedRunId, setSelectedRunId] = useState('');
  const [runDetail, setRunDetail] = useState<WorkflowRun | null>(null);
  const [runEvents, setRunEvents] = useState<WorkflowEvent[]>([]);
  const [runInput, setRunInput] = useState('');
  const [runReply, setRunReply] = useState('');
  const [busyAction, setBusyAction] = useState('');
  const [previewState, setPreviewState] = useState<ArtifactPreviewState | null>(null);
  const [mdRender, setMdRender] = useState(true);

  const selectedTemplate = useMemo(
    () => templates.find(item => item.id === selectedTemplateId) || null,
    [selectedTemplateId, templates],
  );

  const takeoverRuns = useMemo(() => runs.filter(run => isTakeoverRun(run)), [runs]);
  const activeTakeovers = useMemo(() => takeoverRuns.filter(run => isTakeoverActive(run)), [takeoverRuns]);

  useEffect(() => {
    void loadAll();
  }, []);

  useEffect(() => {
    if (!selectedTemplate) return;
    setTemplateDraft(selectedTemplate);
    setTemplateDefinitionText(JSON.stringify(selectedTemplate.definition || { nodes: [], edges: [] }, null, 2));
  }, [selectedTemplate]);

  useEffect(() => {
    if (!selectedRunId) {
      setRunDetail(null);
      setRunEvents([]);
      setPreviewState(null);
      return;
    }
    void loadRunDetail(selectedRunId);
  }, [selectedRunId]);

  useEffect(() => {
    const tick = () => {
      if (document.hidden) return;
      void loadRuns();
      if (selectedRunId) {
        void loadRunDetail(selectedRunId);
      }
    };
    const timer = window.setInterval(tick, 3000);
    const onVisible = () => { if (!document.hidden) tick(); };
    document.addEventListener('visibilitychange', onVisible);
    return () => { window.clearInterval(timer); document.removeEventListener('visibilitychange', onVisible); };
  }, [selectedRunId, runFilter]);

  const pushMessage = (text: string) => {
    setMessage(text);
    window.setTimeout(() => setMessage(''), 2500);
  };

  const loadAll = async () => {
    setLoading(true);
    try {
      const [settingsRes, templatesRes, runsRes, modelsRes] = await Promise.all([
        api.getWorkflowSettings(),
        api.getWorkflowTemplates(),
        api.getWorkflowRuns(runFilter || undefined),
        api.getModels(),
      ]);

      if (settingsRes?.ok) {
        setSettings({ ...DEFAULT_SETTINGS, ...(settingsRes.settings || {}) });
      }
      if (templatesRes?.ok) {
        const nextTemplates = templatesRes.templates || [];
        setTemplates(nextTemplates);
        setSelectedTemplateId(prev => prev || nextTemplates[0]?.id || '');
      }
      if (runsRes?.ok) {
        const nextRuns = runsRes.runs || [];
        setRuns(nextRuns);
        setSelectedRunId(prev => prev || nextRuns[0]?.id || '');
      }
      if (modelsRes?.ok) {
        setModelOptions(extractModelOptions(modelsRes));
      }
    } catch {
      pushMessage('加载工作流数据失败');
    } finally {
      setLoading(false);
    }
  };

  const loadRuns = async (status = runFilter) => {
    try {
      const response = await api.getWorkflowRuns(status || undefined);
      if (response?.ok) {
        const nextRuns = response.runs || [];
        setRuns(nextRuns);
        setSelectedRunId(prev => (prev && nextRuns.some((item: WorkflowRun) => item.id === prev) ? prev : nextRuns[0]?.id || ''));
      }
    } catch {
      pushMessage('刷新运行记录失败');
    }
  };

  const loadRunDetail = async (runId: string) => {
    try {
      const response = await api.getWorkflowRun(runId);
      if (response?.ok) {
        setRunDetail(response.run || null);
        setRunEvents(response.events || []);
      }
    } catch {
      pushMessage('加载运行详情失败');
    }
  };

  const resetTemplateDraft = () => {
    setSelectedTemplateId('');
    setTemplateDraft({
      id: '',
      name: '',
      description: '',
      category: '',
      status: 'ready',
      triggerMode: 'manual',
      settings: {
        approvalMode: settings.approvalMode,
        progressMode: settings.progressMode,
        tone: settings.tone,
      },
      definition: { nodes: [], edges: [] },
    });
    setTemplateDefinitionText(DEFAULT_TEMPLATE_JSON);
  };

  const saveSettings = async () => {
    setSaving(true);
    try {
      const response = await api.updateWorkflowSettings(settings);
      if (response?.ok) {
        pushMessage('工作流设置已保存');
      } else {
        pushMessage(response?.error || '保存设置失败');
      }
    } catch {
      pushMessage('保存设置失败');
    } finally {
      setSaving(false);
    }
  };

  const saveTemplate = async () => {
    let parsedDefinition: Record<string, any>;
    try {
      parsedDefinition = JSON.parse(templateDefinitionText);
    } catch {
      pushMessage('模板定义 JSON 格式不正确');
      return;
    }
    if (!templateDraft.name.trim()) {
      pushMessage('请先填写模板名称');
      return;
    }
    setSaving(true);
    try {
      const payload = {
        ...templateDraft,
        definition: parsedDefinition,
      };
      const response = await api.saveWorkflowTemplate(payload);
      if (response?.ok) {
        pushMessage('模板已保存');
        await loadAll();
        if (response.template?.id) {
          setSelectedTemplateId(response.template.id);
        }
      } else {
        pushMessage(response?.error || '保存模板失败');
      }
    } catch {
      pushMessage('保存模板失败');
    } finally {
      setSaving(false);
    }
  };

  const removeTemplate = async () => {
    if (!selectedTemplateId) return;
    if (!window.confirm('确定删除当前模板？')) return;
    setSaving(true);
    try {
      const response = await api.deleteWorkflowTemplate(selectedTemplateId);
      if (response?.ok) {
        pushMessage('模板已删除');
        resetTemplateDraft();
        await loadAll();
      } else {
        pushMessage(response?.error || '删除模板失败');
      }
    } catch {
      pushMessage('删除模板失败');
    } finally {
      setSaving(false);
    }
  };

  const generateTemplate = async () => {
    if (!generatePrompt.trim()) {
      pushMessage('请先输入一句需求');
      return;
    }
    setGenerating(true);
    try {
      const response = await api.generateWorkflowTemplate(generatePrompt, templateDraft.category || 'AI 生成', {
        approvalMode: settings.approvalMode,
        progressMode: settings.progressMode,
        tone: settings.tone,
      });
      if (response?.ok && response.template) {
        const generated = response.template as WorkflowTemplate;
        setTemplateDraft(generated);
        setSelectedTemplateId('');
        setTemplateDefinitionText(JSON.stringify(generated.definition || { nodes: [], edges: [] }, null, 2));
        pushMessage('已生成模板草稿');
      } else {
        pushMessage(response?.error || 'AI 生成失败');
      }
    } catch {
      pushMessage('AI 生成失败');
    } finally {
      setGenerating(false);
    }
  };

  const startRun = async () => {
    const templateId = selectedTemplateId || templateDraft.id;
    if (!templateId) {
      pushMessage('请先保存模板再启动');
      return;
    }
    setBusyAction('start');
    try {
      const response = await api.startWorkflowRun(templateId, { input: runInput });
      if (response?.ok && response.run?.id) {
        pushMessage('工作流已启动');
        await loadRuns();
        setSelectedRunId(response.run.id);
        setRunInput('');
      } else {
        pushMessage(response?.error || '启动工作流失败');
      }
    } catch {
      pushMessage('启动工作流失败');
    } finally {
      setBusyAction('');
    }
  };

  const controlRun = async (action: string) => {
    if (!selectedRunId) return;
    setBusyAction(action);
    try {
      const response = await api.controlWorkflowRun(selectedRunId, action, runReply || undefined);
      if (response?.ok) {
        pushMessage(`已发送 ${action} 指令`);
        setRunReply('');
        await loadRuns();
        await loadRunDetail(selectedRunId);
      } else {
        pushMessage(response?.error || '运行控制失败');
      }
    } catch {
      pushMessage('运行控制失败');
    } finally {
      setBusyAction('');
    }
  };

  const deleteRun = async () => {
    if (!selectedRunId) return;
    if (!window.confirm('确定彻底删除这个工作流实例？删除后步骤和事件记录也会一起清除。')) return;
    setBusyAction('delete');
    try {
      const response = await api.deleteWorkflowRun(selectedRunId);
      if (response?.ok) {
        pushMessage('工作流实例已删除');
        setSelectedRunId('');
        setRunDetail(null);
        setRunEvents([]);
        await loadRuns();
      } else {
        pushMessage(response?.error || '删除工作流实例失败');
      }
    } catch {
      pushMessage('删除工作流实例失败');
    } finally {
      setBusyAction('');
    }
  };

  const resendArtifact = async (artifact: WorkflowArtifact) => {
	if (!selectedRunId) return;
	setBusyAction(`resend:${artifact.stepKey || artifact.fileName || ''}`);
	try {
	  const response = await api.resendWorkflowArtifact(selectedRunId, { stepKey: artifact.stepKey, fileName: artifact.fileName });
	  if (response?.ok) {
	    pushMessage(`已重新发送 ${artifact.artifactName || artifact.fileName || '文件'}`);
	    await loadRuns();
	    await loadRunDetail(selectedRunId);
	  } else {
	    pushMessage(response?.error || '重新发送失败');
	  }
	} catch {
	  pushMessage('重新发送失败');
	} finally {
	  setBusyAction('');
	}
  };

  const resendFinalArtifacts = async () => {
	if (!selectedRunId) return;
	setBusyAction('resend-final');
	try {
	  const response = await api.resendWorkflowArtifact(selectedRunId, {});
	  if (response?.ok) {
	    pushMessage('已批量重新发送最终文件');
	    await loadRuns();
	    await loadRunDetail(selectedRunId);
	  } else {
	    pushMessage(response?.error || '批量重新发送失败');
	  }
	} catch {
	  pushMessage('批量重新发送失败');
	} finally {
	  setBusyAction('');
	}
  };

  const previewArtifactInline = async (artifact: WorkflowArtifact) => {
	if (!artifact.relativePath) return;
	setBusyAction(`preview:${artifact.stepKey || artifact.fileName || ''}`);
	try {
	  const response = await api.workspacePreview(artifact.relativePath);
	  if (response?.ok && response.type === 'text') {
	    setMdRender(true);
	    setPreviewState({
	      path: artifact.relativePath,
	      title: artifact.artifactName || artifact.fileName || artifact.relativePath,
	      content: String(response.content || ''),
	      isMarkdown: isMarkdownArtifact(artifact.relativePath, artifact.fileName),
	      truncated: Boolean(response.truncated),
	    });
	  } else {
	    pushMessage(response?.error || '暂不支持内嵌预览此文件');
	  }
	} catch {
	  pushMessage('加载预览失败');
	} finally {
	  setBusyAction('');
	}
  };

  const selectedModelValue = settings.providerId && settings.modelId ? `${settings.providerId}/${settings.modelId}` : '';

  return (
    <div className={`space-y-6 ${modern ? 'page-modern' : ''}`}>
      <div className={`${modern ? 'page-modern-header' : 'flex items-center justify-between gap-4'}`}>
        <div>
          <h2 className={`${modern ? 'page-modern-title' : 'text-xl font-bold text-gray-900 dark:text-white'}`}>工作流中心</h2>
          <p className={`${modern ? 'page-modern-subtitle' : 'text-sm text-gray-500 dark:text-gray-400'}`}>配置 Workflow AI、管理模板，并追踪每次运行的进度。</p>
        </div>
        <MobileActionTray label="工作流操作">
          <button onClick={() => void loadAll()} className={`${modern ? 'page-modern-action' : 'px-3 py-2 rounded-lg border'} flex items-center gap-2 text-sm`}>
            <RefreshCw size={14} />刷新
          </button>
          <button onClick={resetTemplateDraft} className={`${modern ? 'page-modern-action' : 'px-3 py-2 rounded-lg border'} flex items-center gap-2 text-sm`}>
            <Wand2 size={14} />新建模板
          </button>
          <button onClick={saveSettings} disabled={saving} className={`${modern ? 'page-modern-accent' : 'px-3 py-2 rounded-lg bg-blue-600 text-white'} flex items-center gap-2 text-sm disabled:opacity-60`}>
            {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}保存设置
          </button>
        </MobileActionTray>
      </div>

      {message && (
        <div className="page-modern-panel px-4 py-3 text-sm font-medium text-slate-700 dark:text-slate-200">
          {message}
        </div>
      )}

      {previewState && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4" onClick={() => setPreviewState(null)}>
          <div className="bg-[linear-gradient(145deg,rgba(255,255,255,0.92),rgba(239,246,255,0.72))] dark:bg-[linear-gradient(145deg,rgba(12,24,42,0.92),rgba(30,64,175,0.14))] rounded-2xl shadow-2xl max-w-4xl w-full max-h-[85vh] overflow-hidden flex flex-col border border-blue-100/70 dark:border-blue-800/20 backdrop-blur-xl" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-5 py-4 border-b border-blue-100/70 dark:border-blue-800/20 bg-[linear-gradient(145deg,rgba(255,255,255,0.82),rgba(239,246,255,0.6))] dark:bg-[linear-gradient(145deg,rgba(10,20,36,0.86),rgba(30,64,175,0.1))]">
              <div className="flex items-center gap-3 min-w-0">
                <div className="p-1.5 rounded-xl bg-blue-100/80 dark:bg-blue-900/20 text-blue-600 dark:text-blue-300 border border-blue-100/70 dark:border-blue-800/30">
                  <Eye size={16} />
                </div>
                <span className="text-sm font-bold text-slate-900 dark:text-white truncate">{previewState.title}</span>
              </div>
              <div className="flex items-center gap-3 shrink-0">
                {previewState.isMarkdown && (
                  <div className="flex items-center rounded-xl border border-blue-100/70 dark:border-blue-800/20 overflow-hidden bg-[linear-gradient(145deg,rgba(255,255,255,0.82),rgba(239,246,255,0.64))] dark:bg-[linear-gradient(145deg,rgba(10,20,36,0.82),rgba(30,64,175,0.08))] shadow-sm backdrop-blur-xl">
                    <button onClick={() => setMdRender(true)} className={`px-3 py-1.5 text-xs font-medium transition-colors ${mdRender ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300' : 'text-slate-500 hover:bg-slate-50 dark:hover:bg-slate-700'}`}>
                      <Eye size={12} className="inline mr-1.5" />渲染
                    </button>
                    <div className="w-px bg-slate-200 dark:bg-slate-700 self-stretch"></div>
                    <button onClick={() => setMdRender(false)} className={`px-3 py-1.5 text-xs font-medium transition-colors ${!mdRender ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300' : 'text-slate-500 hover:bg-slate-50 dark:hover:bg-slate-700'}`}>
                      <FileCode size={12} className="inline mr-1.5" />源码
                    </button>
                  </div>
                )}
                <a href={artifactFileDownloadUrl(previewState.path)} className="page-modern-accent px-3 py-1.5 text-xs flex items-center gap-1.5">
                  <Download size={12} />下载
                </a>
                <button onClick={() => setPreviewState(null)} className="page-modern-action p-1.5">
                  <X size={18} />
                </button>
              </div>
            </div>
            <div className="flex-1 overflow-auto p-6 bg-gray-50/30 dark:bg-black/20">
              {previewState.isMarkdown && mdRender ? (
                <div className="prose prose-sm dark:prose-invert max-w-none bg-white dark:bg-gray-900 p-6 rounded-xl shadow-sm border border-gray-100 dark:border-gray-800" dangerouslySetInnerHTML={{ __html: simpleMarkdown(previewState.content || '') }} />
              ) : (
                <pre className="text-xs font-mono whitespace-pre-wrap break-all text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-100 dark:border-gray-800 shadow-sm h-full overflow-auto">{previewState.content}</pre>
              )}
              {previewState.truncated && <div className="mt-3 text-[11px] text-amber-600 dark:text-amber-300">文件内容较大，当前只展示了前一部分。</div>}
            </div>
          </div>
        </div>
      )}

      {loading ? (
        <div className="page-modern-panel flex items-center justify-center gap-3 px-6 py-20 text-slate-500 dark:text-slate-300">
          <Loader2 size={18} className="animate-spin" />
          <span>正在加载工作流中心…</span>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
            <div className="xl:col-span-2 page-modern-panel p-6 space-y-5">
              <div className="flex items-center gap-3">
                <div className="rounded-2xl bg-blue-100 p-2 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300">
                  <Bot size={18} />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-slate-900 dark:text-white">Workflow AI 设置</h3>
                  <p className="text-sm text-slate-500 dark:text-slate-400">这里决定复杂任务识别、模板生成、进度播报与审批判断的统一行为。</p>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <label className="space-y-2 text-sm">
                  <span className="text-slate-600 dark:text-slate-300">启用工作流拦截</span>
                  <select value={settings.enabled ? 'true' : 'false'} onChange={e => setSettings(prev => ({ ...prev, enabled: e.target.value === 'true' }))} className="page-modern-control w-full">
                    <option value="true">开启</option>
                    <option value="false">关闭</option>
                  </select>
                </label>
                <label className="space-y-2 text-sm">
                  <span className="text-slate-600 dark:text-slate-300">Workflow 模型</span>
                  <select
                    value={selectedModelValue}
                    onChange={e => {
                      const [providerId, modelId] = e.target.value.split('/');
                      setSettings(prev => ({ ...prev, providerId: providerId || '', modelId: modelId || '' }));
                    }}
                    className="page-modern-control w-full"
                  >
                    <option value="">跟随系统默认模型</option>
                    {modelOptions.map(option => (
                      <option key={option.value} value={option.value}>{option.label}</option>
                    ))}
                  </select>
                </label>
                <label className="space-y-2 text-sm">
                  <span className="text-slate-600 dark:text-slate-300">审批级别</span>
                  <select value={String(settings.approvalMode)} onChange={e => setSettings(prev => ({ ...prev, approvalMode: Number(e.target.value) }))} className="page-modern-control w-full">
                    {[1, 2, 3, 4, 5].map(level => <option key={level} value={level}>{level} 级</option>)}
                  </select>
                </label>
                <label className="space-y-2 text-sm">
                  <span className="text-slate-600 dark:text-slate-300">进度模式</span>
                  <select value={settings.progressMode} onChange={e => setSettings(prev => ({ ...prev, progressMode: e.target.value }))} className="page-modern-control w-full">
                    <option value="detailed">逐步播报</option>
                    <option value="concise">关键步骤</option>
                  </select>
                </label>
                <label className="space-y-2 text-sm">
                  <span className="text-slate-600 dark:text-slate-300">消息语气</span>
                  <select value={settings.tone} onChange={e => setSettings(prev => ({ ...prev, tone: e.target.value }))} className="page-modern-control w-full">
                    <option value="professional">专业简洁</option>
                    <option value="operations">运营协作</option>
                    <option value="friendly">友好助手</option>
                  </select>
                </label>
                <label className="space-y-2 text-sm">
                  <span className="text-slate-600 dark:text-slate-300">复杂度阈值</span>
                  <select value={settings.complexityGuard} onChange={e => setSettings(prev => ({ ...prev, complexityGuard: e.target.value }))} className="page-modern-control w-full">
                    <option value="light">轻量拦截</option>
                    <option value="balanced">平衡判断</option>
                    <option value="strict">严格识别</option>
                  </select>
                </label>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <label className="page-modern-panel flex items-center justify-between px-4 py-3 text-sm">
                  <span>复杂任务自动创建运行实例</span>
                  <input type="checkbox" checked={settings.autoCreateRuns} onChange={e => setSettings(prev => ({ ...prev, autoCreateRuns: e.target.checked }))} />
                </label>
                <label className="page-modern-panel flex items-center justify-between px-4 py-3 text-sm">
                  <span>同步推送进度到中心</span>
                  <input type="checkbox" checked={settings.pushProgress} onChange={e => setSettings(prev => ({ ...prev, pushProgress: e.target.checked }))} />
                </label>
              </div>
            </div>

            <div className="page-modern-panel p-6 space-y-4">
              <div>
                <h3 className="text-base font-semibold text-slate-900 dark:text-white">当前策略快照</h3>
                <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">运行开始后，这些执行设置会被冻结在实例里。</p>
              </div>
              <div className="space-y-3 text-sm text-slate-600 dark:text-slate-300">
                <div className="rounded-2xl bg-slate-50 px-4 py-3 dark:bg-slate-900/60">状态：{settings.enabled ? '已开启 Workflow AI 拦截' : '当前直接转发给 OpenClaw'}</div>
                <div className="rounded-2xl bg-slate-50 px-4 py-3 dark:bg-slate-900/60">审批：{settings.approvalMode} 级 / 原会话回复确认</div>
                <div className="rounded-2xl bg-slate-50 px-4 py-3 dark:bg-slate-900/60">进度：{settings.progressMode === 'detailed' ? '每一步' : '关键步骤'}</div>
                <div className="rounded-2xl bg-slate-50 px-4 py-3 dark:bg-slate-900/60">语气：{toneLabel(settings.tone)}</div>
                <div className="rounded-2xl bg-slate-50 px-4 py-3 dark:bg-slate-900/60">自动创建：{settings.autoCreateRuns ? '开启' : '关闭'}</div>
                <div className="rounded-2xl bg-slate-50 px-4 py-3 dark:bg-slate-900/60">当前接管：{activeTakeovers.length} 个运行中 / 等待中，累计 {takeoverRuns.length} 个原会话接管实例</div>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-1 xl:grid-cols-[320px_minmax(0,1fr)] gap-6">
            <div className="page-modern-panel overflow-hidden">
              <div className="border-b border-slate-200/70 px-5 py-4 dark:border-slate-800/70">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <h3 className="text-base font-semibold text-slate-900 dark:text-white">模板列表</h3>
                    <p className="text-sm text-slate-500 dark:text-slate-400">内置模板与可复用模板都在这里。</p>
                  </div>
                  <button onClick={resetTemplateDraft} className="page-modern-action px-3 py-2 text-xs">新建</button>
                </div>
              </div>
              <div className="max-h-[620px] overflow-y-auto p-3 space-y-2">
                {templates.map(item => {
                  const active = item.id === selectedTemplateId;
                  return (
                    <button
                      key={item.id}
                      onClick={() => setSelectedTemplateId(item.id)}
                      className={`w-full rounded-2xl border px-4 py-3 text-left transition-all ${active ? 'border-blue-200 bg-blue-50/70 dark:border-blue-800/40 dark:bg-blue-950/20' : 'border-transparent hover:border-blue-100 hover:bg-white/70 dark:hover:border-blue-900/30 dark:hover:bg-slate-900/40'}`}
                    >
                      <div className="flex items-center justify-between gap-3">
                        <span className="truncate text-sm font-semibold text-slate-900 dark:text-white">{item.name}</span>
                        <span className={`rounded-full px-2 py-0.5 text-[11px] ${statusTone(item.status)}`}>{statusLabel(item.status)}</span>
                      </div>
                      <div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{item.category || '未分类'} · {item.triggerMode || 'manual'}</div>
                    </button>
                  );
                })}
                {templates.length === 0 && <div className="px-4 py-10 text-center text-sm text-slate-400">暂无模板</div>}
              </div>
            </div>

            <div className="page-modern-panel p-6 space-y-5">
              <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_320px] gap-5">
                <div className="space-y-4">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <label className="space-y-2 text-sm">
                      <span>模板名称</span>
                      <input value={templateDraft.name || ''} onChange={e => setTemplateDraft(prev => ({ ...prev, name: e.target.value }))} className="page-modern-control w-full" placeholder="例如：活动运营流程" />
                    </label>
                    <label className="space-y-2 text-sm">
                      <span>模板分类</span>
                      <input value={templateDraft.category || ''} onChange={e => setTemplateDraft(prev => ({ ...prev, category: e.target.value }))} className="page-modern-control w-full" placeholder="例如：运营 / 交付 / 支持" />
                    </label>
                    <label className="space-y-2 text-sm">
                      <span>状态</span>
                      <select value={templateDraft.status || 'ready'} onChange={e => setTemplateDraft(prev => ({ ...prev, status: e.target.value }))} className="page-modern-control w-full">
                        <option value="ready">ready</option>
                        <option value="draft">draft</option>
                        <option value="archived">archived</option>
                      </select>
                    </label>
                    <label className="space-y-2 text-sm">
                      <span>触发方式</span>
                      <select value={templateDraft.triggerMode || 'manual'} onChange={e => setTemplateDraft(prev => ({ ...prev, triggerMode: e.target.value }))} className="page-modern-control w-full">
                        <option value="manual">manual</option>
                        <option value="auto">auto</option>
                      </select>
                    </label>
                  </div>

                  <label className="space-y-2 text-sm block">
                    <span>描述</span>
                    <textarea value={templateDraft.description || ''} onChange={e => setTemplateDraft(prev => ({ ...prev, description: e.target.value }))} rows={3} className="page-modern-control w-full" placeholder="描述这个工作流适合处理什么任务。" />
                  </label>

                  <label className="space-y-2 text-sm block">
                    <span>模板定义 JSON</span>
                    <textarea value={templateDefinitionText} onChange={e => setTemplateDefinitionText(e.target.value)} rows={16} className="page-modern-control w-full font-mono text-xs" />
                  </label>

                  <div className="flex flex-wrap gap-3">
                    <button onClick={saveTemplate} disabled={saving} className="page-modern-accent px-4 py-2 text-sm disabled:opacity-60 flex items-center gap-2">
                      {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}保存模板
                    </button>
                    <button onClick={removeTemplate} disabled={!selectedTemplateId || saving} className="page-modern-danger px-4 py-2 text-sm disabled:opacity-50 flex items-center gap-2">
                      <Square size={14} />删除模板
                    </button>
                  </div>
                </div>

                <div className="space-y-4">
                  <div className="rounded-3xl border border-blue-100/70 bg-[linear-gradient(145deg,rgba(255,255,255,0.92),rgba(239,246,255,0.82))] p-5 shadow-sm dark:border-blue-900/30 dark:bg-[linear-gradient(145deg,rgba(10,20,36,0.88),rgba(30,64,175,0.16))]">
                    <div className="flex items-center gap-2 text-slate-900 dark:text-white">
                      <Sparkles size={16} className="text-blue-500" />
                      <h4 className="font-semibold">一句话生成模板</h4>
                    </div>
                    <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">输入一个业务目标，Workflow AI 会先给出可编辑模板草稿。</p>
                    <textarea value={generatePrompt} onChange={e => setGeneratePrompt(e.target.value)} rows={5} className="page-modern-control mt-4 w-full" placeholder="例如：帮我生成一个从需求收集、方案拆解、审批、执行到复盘的运营工作流。" />
                    <button onClick={generateTemplate} disabled={generating} className="page-modern-accent mt-4 w-full justify-center px-4 py-2 text-sm disabled:opacity-60 flex items-center gap-2">
                      {generating ? <Loader2 size={14} className="animate-spin" /> : <Sparkles size={14} />}AI 生成草稿
                    </button>
                  </div>

                  <div className="page-modern-panel p-5 space-y-3">
                    <div className="flex items-center gap-2 text-slate-900 dark:text-white">
                      <GitBranch size={16} className="text-blue-500" />
                      <h4 className="font-semibold">快速启动</h4>
                    </div>
                    <textarea value={runInput} onChange={e => setRunInput(e.target.value)} rows={5} className="page-modern-control w-full" placeholder="给当前模板补充一次运行输入，例如任务目标、上下文、交付要求。" />
                    <button onClick={startRun} disabled={busyAction === 'start'} className="page-modern-accent w-full justify-center px-4 py-2 text-sm disabled:opacity-60 flex items-center gap-2">
                      {busyAction === 'start' ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}启动当前模板
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-1 xl:grid-cols-[360px_minmax(0,1fr)] gap-6">
            <div className="page-modern-panel overflow-hidden">
              <div className="border-b border-slate-200/70 px-5 py-4 dark:border-slate-800/70">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <h3 className="text-base font-semibold text-slate-900 dark:text-white">运行实例</h3>
                    <p className="text-sm text-slate-500 dark:text-slate-400">支持同一会话内并行运行多个工作流。</p>
                  </div>
                  <select
                    value={runFilter}
                    onChange={e => {
                      const next = e.target.value;
                      setRunFilter(next);
                      void loadRuns(next);
                    }}
                    className="page-modern-control w-[150px] text-sm"
                  >
                    <option value="">全部状态</option>
                        <option value="running">执行中</option>
                        <option value="waiting_for_user">等待用户</option>
                        <option value="waiting_for_approval">等待审批</option>
                        <option value="paused">已暂停</option>
                        <option value="completed">已完成</option>
                        <option value="cancelled">已取消</option>
                  </select>
                </div>
              </div>
              <div className="max-h-[720px] overflow-y-auto p-3 space-y-2">
                {runs.map(run => {
                  const active = run.id === selectedRunId;
                  const takeover = takeoverBadge(run);
                  const pushLabel = originPushLabel(run);
                  return (
                    <button
                      key={run.id}
                      onClick={() => setSelectedRunId(run.id)}
                      className={`w-full rounded-2xl border px-4 py-3 text-left transition-all ${active ? 'border-blue-200 bg-blue-50/70 dark:border-blue-800/40 dark:bg-blue-950/20' : 'border-transparent hover:border-blue-100 hover:bg-white/70 dark:hover:border-blue-900/30 dark:hover:bg-slate-900/40'}`}
                    >
                      <div className="flex items-center justify-between gap-3">
                        <span className="truncate text-sm font-semibold text-slate-900 dark:text-white">{run.name}</span>
                        <span className={`rounded-full px-2 py-0.5 text-[11px] ${statusTone(run.status)}`}>{statusLabel(run.status)}</span>
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
                        <span>{run.shortId || run.id} · {formatTime(run.updatedAt)}</span>
                        <span className="rounded-full bg-slate-100 px-2 py-0.5 dark:bg-slate-800">{channelLabel(run.channelId)}</span>
                        {takeover && <span className="rounded-full bg-amber-100 px-2 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{takeover}</span>}
                        {pushLabel && <span className={`rounded-full px-2 py-0.5 ${originPushTone(run)}`}>{pushLabel}</span>}
                      </div>
                      {run.conversationId && <div className="mt-2 line-clamp-1 text-[11px] text-slate-400">会话：{run.conversationId}</div>}
                      {run.lastMessage && <div className="mt-2 line-clamp-2 text-xs text-slate-500 dark:text-slate-400">{run.lastMessage}</div>}
                    </button>
                  );
                })}
                {runs.length === 0 && <div className="px-4 py-12 text-center text-sm text-slate-400">暂无运行记录</div>}
              </div>
            </div>

            <div className="page-modern-panel p-6 space-y-5">
              {!runDetail ? (
                <div className="flex min-h-[420px] items-center justify-center text-sm text-slate-400">选择一个运行实例查看步骤、事件与控制入口。</div>
              ) : (
                <>
                  <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                    <div>
                      <div className="flex items-center gap-2">
                        <h3 className="text-lg font-semibold text-slate-900 dark:text-white">{runDetail.name}</h3>
                        <span className={`rounded-full px-2.5 py-1 text-xs ${statusTone(runDetail.status)}`}>{statusLabel(runDetail.status)}</span>
                        {takeoverBadge(runDetail) && <span className="rounded-full bg-amber-100 px-2.5 py-1 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{takeoverBadge(runDetail)}</span>}
                        {originPushLabel(runDetail) && <span className={`rounded-full px-2.5 py-1 text-xs ${originPushTone(runDetail)}`}>{originPushLabel(runDetail)}</span>}
                      </div>
                      <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{runDetail.shortId} · 会话 {runDetail.conversationId || '-'} · 通道 {channelLabel(runDetail.channelId)}</p>
                      {isTakeoverRun(runDetail) && (
                        <div className="mt-3 grid grid-cols-1 md:grid-cols-3 gap-3 text-xs text-slate-500 dark:text-slate-400">
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">来源：原会话复杂任务自动接管</div>
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">用户：{runDetail.userId || '-'}</div>
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">通道：{channelLabel(runDetail.channelId)}</div>
                        </div>
                      )}
                      {originPushLabel(runDetail) && (
                        <div className="mt-3 grid grid-cols-1 md:grid-cols-3 gap-3 text-xs text-slate-500 dark:text-slate-400">
                          <div className={`rounded-2xl px-3 py-2 ${originPushTone(runDetail)}`}>回写状态：{originPushLabel(runDetail)}</div>
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">回写时间：{formatTime(Number(runDetail.context?.originPushUpdatedAt || 0))}</div>
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">最近回写：{String(runDetail.context?.originPushMessage || '-')}</div>
                        </div>
                      )}
                      {originPushStatus(runDetail) === 'failed' && runDetail.context?.originPushError && (
                        <div className="mt-3 rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-600 dark:bg-red-950/20 dark:text-red-300">
                          原通道回写失败：{String(runDetail.context.originPushError)}
                        </div>
                      )}
                      {artifactFilesFromRun(runDetail).length > 0 && (
                        <div className="mt-3 grid grid-cols-1 md:grid-cols-4 gap-3 text-xs text-slate-500 dark:text-slate-400">
                          <div className={`rounded-2xl px-3 py-2 ${artifactDeliveryTone(String(runDetail.context?.artifactDeliveryStatus || ''))}`}>
                            文件回传：{artifactDeliveryLabel(String(runDetail.context?.artifactDeliveryStatus || ''))}
                          </div>
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">目标：{artifactDeliveryTargetLabel(runDetail)}</div>
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">回传时间：{formatTime(Number(runDetail.context?.artifactDeliveryUpdatedAt || 0))}</div>
                          <div className="rounded-2xl bg-slate-50 px-3 py-2 dark:bg-slate-900/60">成功 {Array.isArray(runDetail.context?.artifactDeliverySentFiles) ? runDetail.context.artifactDeliverySentFiles.length : 0} / 失败 {Array.isArray(runDetail.context?.artifactDeliveryFailedFiles) ? runDetail.context.artifactDeliveryFailedFiles.length : 0}</div>
                        </div>
                      )}
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button onClick={() => void controlRun('retry')} disabled={busyAction !== ''} className="page-modern-action px-3 py-2 text-sm disabled:opacity-50 flex items-center gap-2"><RefreshCw size={14} />重试</button>
                      <button onClick={() => void controlRun('pause')} disabled={busyAction !== ''} className="page-modern-action px-3 py-2 text-sm disabled:opacity-50 flex items-center gap-2"><Pause size={14} />暂停</button>
                      <button onClick={() => void controlRun('resume')} disabled={busyAction !== ''} className="page-modern-action px-3 py-2 text-sm disabled:opacity-50 flex items-center gap-2"><Play size={14} />继续</button>
                      <button onClick={() => void controlRun('cancel')} disabled={busyAction !== ''} className="page-modern-danger px-3 py-2 text-sm disabled:opacity-50 flex items-center gap-2"><XCircle size={14} />取消</button>
                      <button onClick={() => void deleteRun()} disabled={busyAction !== ''} className="page-modern-danger px-3 py-2 text-sm disabled:opacity-50 flex items-center gap-2"><Trash2 size={14} />删除</button>
                    </div>
                  </div>

                  <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
                    <div className="space-y-4">
                      <div className="page-modern-panel overflow-hidden">
                        <div className="border-b border-slate-200/70 px-4 py-3 text-sm font-semibold dark:border-slate-800/70">步骤进度</div>
                        <div className="max-h-[320px] overflow-y-auto p-3 space-y-2">
                          {(runDetail.steps || []).map(step => (
                            <div key={step.id} className="rounded-2xl border border-slate-200/70 px-4 py-3 dark:border-slate-800/70">
                              <div className="flex items-center justify-between gap-3">
                                <div>
                                  <div className="text-sm font-semibold text-slate-900 dark:text-white">{step.title || step.stepKey}</div>
                                  <div className="text-xs text-slate-500 dark:text-slate-400">{step.stepType} · {step.stepKey}</div>
                                </div>
                                <span className={`rounded-full px-2 py-0.5 text-[11px] ${statusTone(step.status)}`}>{statusLabel(step.status)}</span>
                              </div>
                              {step.outputText && <div className="mt-2 text-xs text-slate-600 dark:text-slate-300 whitespace-pre-wrap">{step.outputText}</div>}
                              {step.errorText && <div className="mt-2 text-xs text-red-500 whitespace-pre-wrap">{step.errorText}</div>}
                            </div>
                          ))}
                        </div>
                      </div>

                      <div className="page-modern-panel p-4 space-y-3">
                        <div className="text-sm font-semibold text-slate-900 dark:text-white">会话回复 / 审批模拟</div>
                        <textarea value={runReply} onChange={e => setRunReply(e.target.value)} rows={4} className="page-modern-control w-full" placeholder="输入原会话中的用户回复，例如：继续、先放着、重试、修改成发群公告。" />
                        <div className="flex flex-wrap gap-2">
                          <button onClick={() => void controlRun('continue')} disabled={busyAction !== ''} className="page-modern-accent px-3 py-2 text-sm disabled:opacity-50 flex items-center gap-2"><CheckCircle2 size={14} />继续 / 同意</button>
                          <button onClick={() => void controlRun('show_error')} disabled={busyAction !== ''} className="page-modern-action px-3 py-2 text-sm disabled:opacity-50">查看错误</button>
                        </div>
                      </div>

                      {artifactFilesFromRun(runDetail).length > 0 && (
                        <div className="page-modern-panel p-4 space-y-3">
                          <div className="flex items-center justify-between gap-3">
                            <div>
                              <div className="text-sm font-semibold text-slate-900 dark:text-white">产出文件</div>
                              <div className="text-xs text-slate-500 dark:text-slate-400">工作流关键步骤生成的文件会统一保存在工作区目录。</div>
                            </div>
                            <div className="flex items-center gap-2">
                              {finalArtifactFilesFromRun(runDetail).length > 0 && (
                                <button onClick={() => void resendFinalArtifacts()} disabled={busyAction !== ''} className="page-modern-action px-3 py-1.5 text-[11px] flex items-center gap-1.5 disabled:opacity-50">
                                  {busyAction === 'resend-final' ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}重发最终文件
                                </button>
                              )}
                              {runDetail.context?.workflowDirRelative && <div className="text-[11px] text-slate-400">{String(runDetail.context.workflowDirRelative)}</div>}
                            </div>
                          </div>
                          <div className="space-y-2">
                            {artifactFilesFromRun(runDetail).map((artifact, index) => (
                              <div key={`${artifact.stepKey || artifact.fileName || index}`} className="rounded-2xl border border-slate-200/70 px-4 py-3 text-sm dark:border-slate-800/70">
                                <div className="flex flex-wrap items-center justify-between gap-2">
                                  <div className="font-medium text-slate-900 dark:text-white">{artifact.artifactName || artifact.title || artifact.fileName}</div>
                                  <div className="flex flex-wrap items-center gap-2">
                                    <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[10px] text-slate-600 dark:bg-slate-800 dark:text-slate-300">{artifactTypeBadge(artifact.relativePath, artifact.fileName)}</span>
                                    {artifact.isFinalOutput && <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">最终结果</span>}
                                    {artifact.sendToUser && <span className="rounded-full bg-blue-100 px-2 py-0.5 text-[10px] text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">回传用户</span>}
                                    <span className={`rounded-full px-2 py-0.5 text-[10px] ${artifactDeliveryTone(artifact.deliveryStatus)}`}>{artifactDeliveryLabel(artifact.deliveryStatus)}</span>
                                  </div>
                                </div>
                                <div className="mt-2 text-xs text-slate-500 dark:text-slate-400">{artifact.fileName} · {artifact.relativePath}</div>
                                <div className="mt-1 text-[11px] text-slate-400">步骤：{artifact.stepKey || '-'} · 更新时间：{formatTime(artifact.updatedAt)}</div>
                                {(artifact.deliveryTarget || artifact.deliveryUpdatedAt) && <div className="mt-1 text-[11px] text-slate-400">回传目标：{artifact.deliveryMode === 'group' ? 'QQ 群聊' : artifact.deliveryMode === 'private' ? 'QQ 私聊' : '-'} {artifact.deliveryTarget || ''} · 回传时间：{formatTime(artifact.deliveryUpdatedAt)}</div>}
                                {artifact.deliveryError && <div className="mt-2 text-[11px] text-red-500 whitespace-pre-wrap">回传失败：{artifact.deliveryError}</div>}
                                {artifact.relativePath && (
                                  <div className="mt-3 flex flex-wrap gap-2">
                                    {supportsInlinePreview(artifact.relativePath, artifact.fileName) && (
                                      <button onClick={() => void previewArtifactInline(artifact)} disabled={busyAction !== ''} className="page-modern-action px-3 py-1.5 text-[11px] flex items-center gap-1.5 disabled:opacity-50">
                                        {busyAction === `preview:${artifact.stepKey || artifact.fileName || ''}` ? <Loader2 size={12} className="animate-spin" /> : <Eye size={12} />}预览
                                      </button>
                                    )}
                                    <a href={artifactFilePreviewUrl(artifact.relativePath)} target="_blank" rel="noreferrer" className="page-modern-action px-3 py-1.5 text-[11px] flex items-center gap-1.5">
                                      <ExternalLink size={12} />{artifactPreviewLabel(artifact.relativePath, artifact.fileName)}
                                    </a>
                                    <a href={artifactFileDownloadUrl(artifact.relativePath)} target="_blank" rel="noreferrer" className="page-modern-action px-3 py-1.5 text-[11px] flex items-center gap-1.5">
                                      <Download size={12} />下载
                                    </a>
                                    <button onClick={() => void resendArtifact(artifact)} disabled={busyAction !== ''} className="page-modern-action px-3 py-1.5 text-[11px] flex items-center gap-1.5 disabled:opacity-50">
                                      {busyAction === `resend:${artifact.stepKey || artifact.fileName || ''}` ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}重新发送
                                    </button>
                                  </div>
                                )}
                              </div>
                            ))}
                          </div>
                          {finalArtifactFilesFromRun(runDetail).length > 0 && (
                            <div className="rounded-2xl bg-emerald-50 px-4 py-3 text-xs text-emerald-700 dark:bg-emerald-950/20 dark:text-emerald-300">
                              最终将回传给用户的文件：{finalArtifactFilesFromRun(runDetail).map(item => item.fileName || item.artifactName).join('、')}
                            </div>
                          )}
                        </div>
                      )}

                      {runDetail.sourceMessage && (
                        <div className="page-modern-panel p-4 space-y-2">
                          <div className="text-sm font-semibold text-slate-900 dark:text-white">接管来源消息</div>
                          <div className="rounded-2xl bg-slate-50 px-4 py-3 text-sm text-slate-600 dark:bg-slate-900/60 dark:text-slate-300 whitespace-pre-wrap">
                            {runDetail.sourceMessage}
                          </div>
                        </div>
                      )}
                    </div>

                    <div className="page-modern-panel overflow-hidden">
                      <div className="border-b border-slate-200/70 px-4 py-3 text-sm font-semibold dark:border-slate-800/70">事件流</div>
                      <div className="max-h-[480px] overflow-y-auto p-3 space-y-3">
                        {runDetail.lastMessage && (
                          <div className="rounded-2xl bg-blue-50/70 px-4 py-3 text-sm text-slate-700 dark:bg-blue-950/20 dark:text-slate-200">
                            {runDetail.lastMessage}
                          </div>
                        )}
                        {runEvents.map(event => (
                          <div key={event.id} className="rounded-2xl border border-slate-200/70 px-4 py-3 dark:border-slate-800/70">
                            <div className="flex items-center justify-between gap-3">
                              <span className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">{event.eventType}</span>
                              <span className="text-[11px] text-slate-400">{formatTime(event.createdAt)}</span>
                            </div>
                            <div className="mt-2 whitespace-pre-wrap text-sm text-slate-700 dark:text-slate-200">{event.message}</div>
                          </div>
                        ))}
                        {runEvents.length === 0 && <div className="px-4 py-12 text-center text-sm text-slate-400">暂无事件</div>}
                      </div>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
