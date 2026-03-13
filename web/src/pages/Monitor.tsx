import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ReactFlow,
  Controls,
  MiniMap,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type NodeProps,
  Handle,
  Position,
  MarkerType,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import { api } from '../lib/api';
import { useI18n } from '../i18n';
import {
  RefreshCw,
  Radio,
  Bot,
  Star,
  Activity,
  ArrowRightLeft,
  Loader2,
  Info,
  Zap,
  Shield,
  Users,
  Clock,
  Hash,
  MessageSquare,
  ChevronRight,
  Network,
} from 'lucide-react';

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface AgentItem {
  id: string;
  default?: boolean;
  workspace?: string;
  sessions?: number;
  lastActive?: number;
  tools?: {
    profile?: string;
    agentToAgent?: { enabled?: boolean; allow?: string[] };
    sessions?: { visibility?: string };
  };
  identity?: { name?: string; emoji?: string; description?: string; theme?: string };
  sandbox?: { mode?: string; workspaceAccess?: string };
  groupChat?: { enabled?: boolean };
}

interface Binding {
  type?: string;
  agentId: string;
  comment?: string;
  enabled?: boolean;
  match: {
    channel: string;
    peer?: { kind: string; id: string } | string;
    accountId?: string;
    guildId?: string;
    teamId?: string;
    sender?: string;
    roles?: string[];
  };
  acp?: { mode?: string; label?: string };
}

interface ChannelConfig {
  enabled?: boolean;
  type?: string;
  label?: string;
  [key: string]: any;
}

type SelectedItem =
  | { kind: 'agent'; data: AgentItem }
  | { kind: 'channel'; data: { id: string; config: ChannelConfig; linkedAgents: string[] } }
  | { kind: 'binding'; data: Binding & { index: number } }
  | null;

/* ------------------------------------------------------------------ */
/*  Relative time helper                                               */
/* ------------------------------------------------------------------ */

function useRelativeTime() {
  const { t } = useI18n();
  return useCallback(
    (ts?: number) => {
      if (!ts) return t.monitor.never;
      const diff = Math.floor((Date.now() - ts) / 1000);
      if (diff < 10) return t.monitor.justNow;
      if (diff < 60) return `${diff}${t.monitor.secondsAgo}`;
      if (diff < 3600) return `${Math.floor(diff / 60)}${t.monitor.minutesAgo}`;
      if (diff < 86400) return `${Math.floor(diff / 3600)}${t.monitor.hoursAgo}`;
      return `${Math.floor(diff / 86400)}${t.monitor.daysAgo}`;
    },
    [t],
  );
}

/* ------------------------------------------------------------------ */
/*  Custom Nodes                                                       */
/* ------------------------------------------------------------------ */

interface ChannelNodeData {
  label: string;
  channelId: string;
  enabled: boolean;
  channelType: string;
  connected: boolean;
  [key: string]: unknown;
}

const ChannelNode = memo(({ data }: NodeProps<Node<ChannelNodeData>>) => {
  const enabled = data.enabled;
  const connected = data.connected;
  return (
    <div
      className={`
        relative px-4 py-3 rounded-xl border-2 shadow-sm min-w-[160px] transition-all
        bg-white dark:bg-slate-800
        ${enabled ? 'border-emerald-400 dark:border-emerald-500' : 'border-slate-300 dark:border-slate-600'}
      `}
    >
      <Handle type="source" position={Position.Right} className="!w-2.5 !h-2.5 !bg-emerald-500 !border-white dark:!border-slate-800 !border-2" />
      {/* Status dot */}
      <span
        className={`absolute top-2 right-2 w-2.5 h-2.5 rounded-full ${
          connected ? 'bg-emerald-400 animate-pulse' : enabled ? 'bg-amber-400' : 'bg-slate-400'
        }`}
      />
      <div className="flex items-center gap-2.5">
        <div className={`flex items-center justify-center w-8 h-8 rounded-lg ${enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-400' : 'bg-slate-100 dark:bg-slate-700 text-slate-400'}`}>
          <Radio size={16} />
        </div>
        <div className="min-w-0">
          <div className="text-sm font-semibold text-slate-800 dark:text-slate-100 truncate max-w-[120px]">
            {data.label}
          </div>
          <div className="text-[10px] text-slate-400 dark:text-slate-500 uppercase tracking-wider">
            {data.channelType || 'channel'}
          </div>
        </div>
      </div>
    </div>
  );
});
ChannelNode.displayName = 'ChannelNode';

interface AgentNodeData {
  label: string;
  agentId: string;
  emoji: string;
  isDefault: boolean;
  sessions: number;
  lastActiveLabel: string;
  theme: string;
  [key: string]: unknown;
}

const AgentNode = memo(({ data }: NodeProps<Node<AgentNodeData>>) => {
  const isDefault = data.isDefault;
  return (
    <div
      className={`
        relative px-4 py-3 rounded-xl border-2 shadow-sm min-w-[170px] transition-all
        bg-white dark:bg-slate-800
        ${isDefault
          ? 'border-indigo-400 dark:border-indigo-500 ring-2 ring-indigo-200/50 dark:ring-indigo-800/40'
          : 'border-violet-300 dark:border-violet-600'}
      `}
    >
      <Handle type="target" position={Position.Left} className="!w-2.5 !h-2.5 !bg-violet-500 !border-white dark:!border-slate-800 !border-2" />
      <Handle type="source" position={Position.Right} className="!w-2.5 !h-2.5 !bg-amber-500 !border-white dark:!border-slate-800 !border-2" />
      {/* Default star badge */}
      {isDefault && (
        <span className="absolute -top-2 -right-2 flex items-center justify-center w-5 h-5 bg-amber-400 rounded-full shadow-sm">
          <Star size={11} className="text-white" fill="currentColor" />
        </span>
      )}
      <div className="flex items-center gap-2.5">
        <div className="flex items-center justify-center w-9 h-9 rounded-lg bg-violet-50 dark:bg-violet-900/30 text-lg select-none">
          {data.emoji || '🤖'}
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold text-slate-800 dark:text-slate-100 truncate max-w-[110px]">
            {data.label}
          </div>
          <div className="flex items-center gap-2 mt-0.5">
            <span className="inline-flex items-center gap-0.5 text-[10px] bg-violet-100 dark:bg-violet-900/40 text-violet-600 dark:text-violet-300 px-1.5 py-0.5 rounded-full font-medium">
              <MessageSquare size={9} /> {data.sessions ?? 0}
            </span>
            <span className="text-[10px] text-slate-400 dark:text-slate-500 truncate">
              {data.lastActiveLabel}
            </span>
          </div>
        </div>
      </div>
    </div>
  );
});
AgentNode.displayName = 'AgentNode';

const nodeTypes = { channelNode: ChannelNode, agentNode: AgentNode };

/* ------------------------------------------------------------------ */
/*  Dagre Layout                                                       */
/* ------------------------------------------------------------------ */

const NODE_WIDTH = 200;
const NODE_HEIGHT = 70;

function layoutGraph(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: 'LR', ranksep: 180, nodesep: 40, marginx: 40, marginy: 40 });
  g.setDefaultEdgeLabel(() => ({}));

  nodes.forEach((n) => g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT }));
  edges.forEach((e) => g.setEdge(e.source, e.target));
  dagre.layout(g);

  return nodes.map((n) => {
    const pos = g.node(n.id);
    return { ...n, position: { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT / 2 } };
  });
}

/* ------------------------------------------------------------------ */
/*  Build Topology                                                     */
/* ------------------------------------------------------------------ */

function buildTopology(
  agents: AgentItem[],
  bindings: Binding[],
  channels: Record<string, ChannelConfig>,
  _status: any,
  relativeTime: (ts?: number) => string,
): { nodes: Node[]; edges: Edge[] } {
  const rawNodes: Node[] = [];
  const rawEdges: Edge[] = [];

  const defaultAgent = agents.find((a) => a.default) || agents[0];

  // Channel nodes
  Object.entries(channels).forEach(([id, cfg]) => {
    rawNodes.push({
      id: `ch-${id}`,
      type: 'channelNode',
      position: { x: 0, y: 0 },
      data: {
        label: cfg.label || id,
        channelId: id,
        enabled: cfg.enabled !== false,
        channelType: cfg.type || id,
        connected: cfg.enabled !== false,
      },
    });
  });

  // Agent nodes
  agents.forEach((a) => {
    rawNodes.push({
      id: `ag-${a.id}`,
      type: 'agentNode',
      position: { x: 0, y: 0 },
      data: {
        label: a.identity?.name || a.id,
        agentId: a.id,
        emoji: a.identity?.emoji || '🤖',
        isDefault: !!a.default,
        sessions: a.sessions ?? 0,
        lastActiveLabel: relativeTime(a.lastActive),
        theme: a.identity?.theme || 'violet',
      },
    });
  });

  // Binding edges
  const boundChannelIds = new Set<string>();
  bindings.forEach((b, idx) => {
    if (!b.match?.channel || !b.agentId) return;
    const channelNodeId = `ch-${b.match.channel}`;
    const agentNodeId = `ag-${b.agentId}`;
    if (!rawNodes.find((n) => n.id === channelNodeId) || !rawNodes.find((n) => n.id === agentNodeId)) return;
    boundChannelIds.add(b.match.channel);
    rawEdges.push({
      id: `bind-${idx}`,
      source: channelNodeId,
      target: agentNodeId,
      label: b.comment || `#${idx + 1}`,
      type: 'default',
      animated: false,
      style: { stroke: b.enabled === false ? '#94a3b8' : '#6366f1', strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: b.enabled === false ? '#94a3b8' : '#6366f1', width: 16, height: 16 },
      data: { bindingIndex: idx, binding: b },
    });
  });

  // Fallback edges: unbound channels → default agent
  if (defaultAgent) {
    Object.keys(channels).forEach((chId) => {
      if (!boundChannelIds.has(chId)) {
        rawEdges.push({
          id: `fallback-${chId}`,
          source: `ch-${chId}`,
          target: `ag-${defaultAgent.id}`,
          type: 'default',
          animated: false,
          style: { stroke: '#cbd5e1', strokeWidth: 1.5, strokeDasharray: '6 4' },
          markerEnd: { type: MarkerType.ArrowClosed, color: '#cbd5e1', width: 14, height: 14 },
          data: { fallback: true },
        });
      }
    });
  }

  // Sub-agent edges (agent→agent)
  agents.forEach((a) => {
    const allowed = a.tools?.agentToAgent?.allow;
    if (Array.isArray(allowed) && a.tools?.agentToAgent?.enabled !== false) {
      allowed.forEach((targetId) => {
        if (rawNodes.find((n) => n.id === `ag-${targetId}`)) {
          rawEdges.push({
            id: `sub-${a.id}-${targetId}`,
            source: `ag-${a.id}`,
            target: `ag-${targetId}`,
            type: 'default',
            animated: true,
            style: { stroke: '#f59e0b', strokeWidth: 2, strokeDasharray: '5 3' },
            markerEnd: { type: MarkerType.ArrowClosed, color: '#f59e0b', width: 14, height: 14 },
            data: { subagent: true, from: a.id, to: targetId },
          });
        }
      });
    }
  });

  const layoutedNodes = layoutGraph(rawNodes, rawEdges);
  return { nodes: layoutedNodes, edges: rawEdges };
}

/* ------------------------------------------------------------------ */
/*  Detail Panel                                                       */
/* ------------------------------------------------------------------ */

const DetailPanel = memo(({ selected, t }: { selected: SelectedItem; t: any }) => {
  if (!selected) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-center p-6 gap-3">
        <div className="w-12 h-12 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center">
          <Info size={22} className="text-slate-400" />
        </div>
        <p className="text-sm font-medium text-slate-500 dark:text-slate-400">{t.monitor.selectNode}</p>
        <p className="text-xs text-slate-400 dark:text-slate-500">{t.monitor.selectNodeHint}</p>
      </div>
    );
  }

  if (selected.kind === 'agent') {
    const a = selected.data;
    return (
      <div className="space-y-4 p-4 overflow-y-auto h-full">
        <div className="flex items-center gap-3">
          <span className="text-2xl">{a.identity?.emoji || '🤖'}</span>
          <div>
            <h3 className="text-base font-semibold text-slate-800 dark:text-slate-100">{a.identity?.name || a.id}</h3>
            <p className="text-xs text-slate-400">{t.monitor.agentDetail}</p>
          </div>
          {a.default && (
            <span className="ml-auto inline-flex items-center gap-1 text-[10px] bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 px-2 py-0.5 rounded-full font-semibold">
              <Star size={10} fill="currentColor" /> {t.monitor.defaultAgent}
            </span>
          )}
        </div>
        {a.identity?.description && (
          <p className="text-xs text-slate-500 dark:text-slate-400 leading-relaxed">{a.identity.description}</p>
        )}
        <div className="grid grid-cols-2 gap-2">
          <DetailCard icon={<MessageSquare size={13} />} label={t.monitor.sessionCount} value={String(a.sessions ?? 0)} />
          <DetailCard icon={<Shield size={13} />} label={t.monitor.sandboxMode} value={a.sandbox?.mode || 'default'} />
          <DetailCard icon={<Zap size={13} />} label={t.monitor.toolsProfile} value={a.tools?.profile || 'default'} />
          <DetailCard icon={<Users size={13} />} label={t.monitor.groupChat} value={a.groupChat?.enabled ? t.monitor.enabled : t.monitor.disabled} />
        </div>
        {a.tools?.agentToAgent?.allow && a.tools.agentToAgent.allow.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-slate-500 dark:text-slate-400 mb-2 flex items-center gap-1.5">
              <ArrowRightLeft size={12} /> {t.monitor.subagents}
            </h4>
            <div className="flex flex-wrap gap-1.5">
              {a.tools.agentToAgent.allow.map((s) => (
                <span key={s} className="text-[11px] bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 px-2 py-0.5 rounded-md font-medium">
                  {s}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    );
  }

  if (selected.kind === 'channel') {
    const c = selected.data;
    return (
      <div className="space-y-4 p-4 overflow-y-auto h-full">
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-lg bg-emerald-50 dark:bg-emerald-900/30 flex items-center justify-center text-emerald-600 dark:text-emerald-400">
            <Radio size={18} />
          </div>
          <div>
            <h3 className="text-base font-semibold text-slate-800 dark:text-slate-100">{c.config.label || c.id}</h3>
            <p className="text-xs text-slate-400">{t.monitor.channelDetail}</p>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <DetailCard icon={<Hash size={13} />} label="ID" value={c.id} />
          <DetailCard
            icon={<Activity size={13} />}
            label={t.monitor.enabled}
            value={c.config.enabled !== false ? t.monitor.enabled : t.monitor.disabled}
            color={c.config.enabled !== false ? 'emerald' : 'slate'}
          />
        </div>
        {c.linkedAgents.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-slate-500 dark:text-slate-400 mb-2 flex items-center gap-1.5">
              <Bot size={12} /> {t.monitor.targetAgent}
            </h4>
            <div className="flex flex-wrap gap-1.5">
              {c.linkedAgents.map((a) => (
                <span key={a} className="text-[11px] bg-violet-50 dark:bg-violet-900/20 text-violet-700 dark:text-violet-300 px-2 py-0.5 rounded-md font-medium">
                  {a}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    );
  }

  if (selected.kind === 'binding') {
    const b = selected.data;
    return (
      <div className="space-y-4 p-4 overflow-y-auto h-full">
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-lg bg-indigo-50 dark:bg-indigo-900/30 flex items-center justify-center text-indigo-600 dark:text-indigo-400">
            <ArrowRightLeft size={18} />
          </div>
          <div>
            <h3 className="text-base font-semibold text-slate-800 dark:text-slate-100">
              {b.comment || `${t.monitor.agentRoute} #${b.index + 1}`}
            </h3>
            <p className="text-xs text-slate-400">{t.monitor.bindingDetail}</p>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <DetailCard icon={<Radio size={13} />} label={t.monitor.channels} value={b.match.channel} />
          <DetailCard icon={<Bot size={13} />} label={t.monitor.targetAgent} value={b.agentId} />
          <DetailCard icon={<Hash size={13} />} label={t.monitor.bindingType} value={b.type || 'route'} />
          <DetailCard
            icon={<Activity size={13} />}
            label={t.monitor.enabled}
            value={b.enabled !== false ? t.monitor.enabled : t.monitor.disabled}
            color={b.enabled !== false ? 'emerald' : 'slate'}
          />
        </div>
        {b.match.peer && (
          <div className="text-xs text-slate-500 dark:text-slate-400">
            <span className="font-medium">{t.monitor.matchCondition}: </span>
            {typeof b.match.peer === 'string' ? b.match.peer : `${b.match.peer.kind}:${b.match.peer.id}`}
          </div>
        )}
      </div>
    );
  }

  return null;
});
DetailPanel.displayName = 'DetailPanel';

function DetailCard({
  icon,
  label,
  value,
  color,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  color?: string;
}) {
  const colorMap: Record<string, string> = {
    emerald: 'text-emerald-600 dark:text-emerald-400',
    slate: 'text-slate-400 dark:text-slate-500',
  };
  return (
    <div className="bg-slate-50 dark:bg-slate-700/50 rounded-lg px-3 py-2">
      <div className="flex items-center gap-1 text-[10px] text-slate-400 dark:text-slate-500 font-medium mb-0.5">
        {icon} {label}
      </div>
      <div className={`text-xs font-semibold truncate ${color ? colorMap[color] ?? '' : 'text-slate-700 dark:text-slate-200'}`}>
        {value}
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Metric Card                                                        */
/* ------------------------------------------------------------------ */

function MetricCard({ icon, label, value, color }: { icon: React.ReactNode; label: string; value: number; color: string }) {
  const colors: Record<string, string> = {
    violet: 'bg-violet-50 dark:bg-violet-900/20 text-violet-600 dark:text-violet-400',
    emerald: 'bg-emerald-50 dark:bg-emerald-900/20 text-emerald-600 dark:text-emerald-400',
    indigo: 'bg-indigo-50 dark:bg-indigo-900/20 text-indigo-600 dark:text-indigo-400',
    amber: 'bg-amber-50 dark:bg-amber-900/20 text-amber-600 dark:text-amber-400',
  };
  return (
    <div className="bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 px-4 py-3 flex items-center gap-3 shadow-sm">
      <div className={`flex items-center justify-center w-9 h-9 rounded-lg ${colors[color] || colors.violet}`}>
        {icon}
      </div>
      <div>
        <div className="text-xl font-bold text-slate-800 dark:text-slate-100 leading-none">{value}</div>
        <div className="text-[11px] text-slate-400 dark:text-slate-500 font-medium mt-0.5">{label}</div>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Agent Status Card                                                  */
/* ------------------------------------------------------------------ */

function AgentStatusCard({ agent, relativeTime, t }: { agent: AgentItem; relativeTime: (ts?: number) => string; t: any }) {
  return (
    <div className="flex-shrink-0 w-[140px] bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 p-3 shadow-sm hover:shadow-md transition-shadow">
      <div className="flex items-center gap-2 mb-2">
        <span className="text-lg select-none">{agent.identity?.emoji || '🤖'}</span>
        <span className="text-xs font-semibold text-slate-700 dark:text-slate-200 truncate flex-1">
          {agent.identity?.name || agent.id}
        </span>
        {agent.default && <Star size={11} className="text-amber-400 flex-shrink-0" fill="currentColor" />}
      </div>
      <div className="flex items-center justify-between text-[10px]">
        <span className="flex items-center gap-0.5 text-violet-500 dark:text-violet-400 font-medium">
          <MessageSquare size={9} /> {agent.sessions ?? 0}
        </span>
        <span className="text-slate-400 dark:text-slate-500 truncate ml-1">
          {relativeTime(agent.lastActive)}
        </span>
      </div>
      <div className="mt-1.5 flex items-center gap-1">
        <span className={`w-1.5 h-1.5 rounded-full ${agent.lastActive && Date.now() - agent.lastActive < 300000 ? 'bg-emerald-400' : 'bg-slate-300 dark:bg-slate-600'}`} />
        <span className="text-[9px] text-slate-400 dark:text-slate-500">
          {agent.lastActive && Date.now() - agent.lastActive < 300000 ? t.monitor.connected : t.monitor.lastActive}
        </span>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main Component                                                     */
/* ------------------------------------------------------------------ */

function MonitorPage() {
  const { t } = useI18n();
  const relativeTime = useRelativeTime();
  const reactFlowRef = useRef<any>(null);

  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [agents, setAgents] = useState<AgentItem[]>([]);
  const [bindings, setBindings] = useState<Binding[]>([]);
  const [channels, setChannels] = useState<Record<string, ChannelConfig>>({});
  const [status, setStatus] = useState<any>(null);
  const [selected, setSelected] = useState<SelectedItem>(null);

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);

  // Stable ref for relativeTime to avoid re-creating loadData on language change
  const relativeTimeRef = useRef(relativeTime);
  relativeTimeRef.current = relativeTime;

  /* ---- Data fetch ---- */
  const loadData = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    try {
      const [agRes, biRes, chRes, stRes] = await Promise.all([
        api.getAgentsConfig(),
        api.getBindings(),
        api.getChannels(),
        api.getStatus(),
      ]);
      const agentsList: AgentItem[] = agRes.ok
        ? (agRes as any).agents?.list ?? (agRes as any).agents ?? []
        : [];
      const bindingsList: Binding[] = biRes.ok ? (biRes as any).bindings ?? [] : [];
      const channelsMap: Record<string, ChannelConfig> = chRes.ok ? (chRes as any).channels ?? {} : {};
      const statusObj = stRes.ok ? stRes : null;

      setAgents(agentsList);
      setBindings(bindingsList);
      setChannels(channelsMap);
      setStatus(statusObj);

      const topo = buildTopology(agentsList, bindingsList, channelsMap, statusObj, relativeTimeRef.current);
      setNodes(topo.nodes);
      setEdges(topo.edges);
    } catch (err) {
      console.error('Monitor loadData error:', err);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [setNodes, setEdges]);

  useEffect(() => { loadData(); }, [loadData]);

  const handleRefresh = useCallback(() => loadData(true), [loadData]);

  /* ---- Selection handlers ---- */
  const onNodeClick = useCallback(
    (_: any, node: Node) => {
      if (node.type === 'agentNode') {
        const agent = agents.find((a) => a.id === (node.data as any).agentId);
        if (agent) setSelected({ kind: 'agent', data: agent });
      } else if (node.type === 'channelNode') {
        const chId = (node.data as any).channelId as string;
        const linked = bindings.filter((b) => b.match.channel === chId).map((b) => b.agentId);
        setSelected({ kind: 'channel', data: { id: chId, config: channels[chId] || {}, linkedAgents: [...new Set(linked)] } });
      }
    },
    [agents, bindings, channels],
  );

  const onEdgeClick = useCallback(
    (_: any, edge: Edge) => {
      const d = edge.data as any;
      if (d?.binding) {
        setSelected({ kind: 'binding', data: { ...d.binding, index: d.bindingIndex ?? 0 } });
      }
    },
    [],
  );

  const onPaneClick = useCallback(() => setSelected(null), []);

  /* ---- Metrics ---- */
  const totalSessions = useMemo(() => agents.reduce((s, a) => s + (a.sessions ?? 0), 0), [agents]);
  const totalChannels = useMemo(() => Object.keys(channels).length, [channels]);

  /* ---- Fit view callback ---- */
  const onInit = useCallback((instance: any) => {
    reactFlowRef.current = instance;
    setTimeout(() => instance.fitView({ padding: 0.15 }), 100);
  }, []);

  /* ---- Loading state ---- */
  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-[70vh] gap-4">
        <Loader2 size={36} className="text-indigo-500 animate-spin" />
        <p className="text-sm text-slate-500 dark:text-slate-400 animate-pulse">{t.monitor.refreshing}</p>
      </div>
    );
  }

  /* ---- Empty state ---- */
  if (agents.length === 0 && Object.keys(channels).length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-[70vh] gap-4">
        <div className="w-16 h-16 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center">
          <Network size={28} className="text-slate-400" />
        </div>
        <h3 className="text-base font-semibold text-slate-700 dark:text-slate-200">{t.monitor.noData}</h3>
        <p className="text-sm text-slate-400 dark:text-slate-500 max-w-xs text-center">{t.monitor.noDataHint}</p>
        <button
          onClick={handleRefresh}
          className="mt-2 inline-flex items-center gap-2 px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium rounded-lg transition-colors"
        >
          <RefreshCw size={14} /> {t.monitor.refresh}
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4 h-full flex flex-col">
      {/* ── Header ── */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-bold text-slate-800 dark:text-slate-100 flex items-center gap-2">
            <Network size={20} className="text-indigo-500" /> {t.monitor.title}
          </h1>
          <p className="text-xs text-slate-400 dark:text-slate-500 mt-0.5">{t.monitor.subtitle}</p>
        </div>
        <button
          onClick={handleRefresh}
          disabled={refreshing}
          className="inline-flex items-center gap-2 px-3.5 py-2 bg-indigo-600 hover:bg-indigo-700 disabled:opacity-60 text-white text-sm font-medium rounded-lg transition-colors shadow-sm"
        >
          <RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />
          {refreshing ? t.monitor.refreshing : t.monitor.refresh}
        </button>
      </div>

      {/* ── Metrics Bar ── */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <MetricCard icon={<Bot size={18} />} label={t.monitor.agents} value={agents.length} color="violet" />
        <MetricCard icon={<Radio size={18} />} label={t.monitor.channels} value={totalChannels} color="emerald" />
        <MetricCard icon={<ArrowRightLeft size={18} />} label={t.monitor.bindings} value={bindings.length} color="indigo" />
        <MetricCard icon={<Activity size={18} />} label={t.monitor.sessions} value={totalSessions} color="amber" />
      </div>

      {/* ── Main Area: Canvas + Detail Panel ── */}
      <div className="flex-1 flex flex-col lg:flex-row gap-3 min-h-0" style={{ minHeight: '420px' }}>
        {/* Canvas */}
        <div className="flex-[7] bg-white dark:bg-slate-800/80 rounded-xl border border-slate-200 dark:border-slate-700 shadow-sm overflow-hidden relative">
          <div className="absolute top-3 left-3 z-10 flex items-center gap-1.5 text-[10px] text-slate-400 dark:text-slate-500 font-medium bg-white/80 dark:bg-slate-800/80 backdrop-blur px-2 py-1 rounded-md">
            <Network size={11} /> {t.monitor.topology}
          </div>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onNodeClick={onNodeClick}
            onEdgeClick={onEdgeClick}
            onPaneClick={onPaneClick}
            onInit={onInit}
            nodeTypes={nodeTypes}
            fitView
            fitViewOptions={{ padding: 0.15 }}
            minZoom={0.3}
            maxZoom={2}
            proOptions={{ hideAttribution: true }}
          >
            <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="#cbd5e1" className="dark:!bg-slate-900 !bg-slate-50" />
            <Controls position="bottom-right" className="!rounded-lg !border-slate-200 dark:!border-slate-700 !shadow-sm [&>button]:!rounded-md [&>button]:!border-slate-200 dark:[&>button]:!border-slate-600 dark:[&>button]:!bg-slate-800 dark:[&>button]:!text-slate-300" />
            <MiniMap
              position="bottom-left"
              nodeColor={(n) => (n.type === 'agentNode' ? '#8b5cf6' : '#34d399')}
              maskColor="rgba(241,245,249,0.7)"
              className="!rounded-lg !border-slate-200 dark:!border-slate-700 !shadow-sm"
              pannable
              zoomable
            />
          </ReactFlow>
          {/* Legend */}
          <div className="absolute bottom-3 right-14 z-10 flex items-center gap-3 text-[9px] text-slate-400 dark:text-slate-500 bg-white/90 dark:bg-slate-800/90 backdrop-blur px-2.5 py-1.5 rounded-md border border-slate-200/50 dark:border-slate-700/50">
            <span className="flex items-center gap-1"><span className="w-4 h-0.5 bg-indigo-500 rounded" /> {t.monitor.agentRoute}</span>
            <span className="flex items-center gap-1"><span className="w-4 h-0.5 bg-slate-300 rounded" style={{ backgroundImage: 'repeating-linear-gradient(90deg, #cbd5e1 0 4px, transparent 4px 7px)' }} /> {t.monitor.fallbackRoute}</span>
            <span className="flex items-center gap-1"><span className="w-4 h-0.5 bg-amber-500 rounded" style={{ backgroundImage: 'repeating-linear-gradient(90deg, #f59e0b 0 3px, transparent 3px 6px)' }} /> {t.monitor.subagentCall}</span>
          </div>
        </div>

        {/* Detail Panel */}
        <div className="flex-[3] bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 shadow-sm overflow-hidden min-w-[260px]">
          <div className="flex items-center gap-1.5 px-4 py-2.5 border-b border-slate-100 dark:border-slate-700 text-xs font-semibold text-slate-500 dark:text-slate-400">
            <ChevronRight size={13} /> {t.monitor.details}
          </div>
          <DetailPanel selected={selected} t={t} />
        </div>
      </div>

      {/* ── Agent Status Cards ── */}
      {agents.length > 0 && (
        <div>
          <h3 className="text-xs font-semibold text-slate-500 dark:text-slate-400 mb-2 flex items-center gap-1.5">
            <Clock size={12} /> {t.monitor.statusCards}
          </h3>
          <div className="flex gap-2.5 overflow-x-auto pb-2 scrollbar-thin scrollbar-thumb-slate-200 dark:scrollbar-thumb-slate-700">
            {agents.map((a) => (
              <AgentStatusCard key={a.id} agent={a} relativeTime={relativeTime} t={t} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export default memo(MonitorPage);
