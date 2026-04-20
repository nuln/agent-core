import React, { useEffect, useState } from 'react';
import axios from 'axios';
import { Cpu, MessageSquare, Zap, Activity, HardDrive } from 'lucide-react';

interface ComponentInfo {
  name: string;
  description: string;
}

interface SkillInfo {
  name: string;
  description: string;
  manager: string;
}

interface EngineStatus {
  dialogs: ComponentInfo[];
  llms: ComponentInfo[];
  skills: SkillInfo[];
}

const Dashboard: React.FC = () => {
  const [status, setStatus] = useState<EngineStatus | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    axios.get('/api/v1/status')
      .then(res => setStatus(res.data))
      .catch(err => console.error('Failed to load engine status', err))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="text-[var(--text-tertiary)] flex items-center gap-2"><Activity size={16} className="animate-spin" /> Syncing engine status...</div>;
  if (!status) return <div className="text-[var(--color-error)]">Failed to connect to AI Agent core.</div>;

  return (
    <div className="space-y-10 animate-in fade-in duration-500">
      {/* Header & Global Stats */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6">
        <div>
          <h1 className="text-4xl font-bold tracking-tight text-[var(--text-primary)]">System Overview</h1>
          <p className="text-[var(--text-secondary)] mt-2 font-medium">Real-time status of your autonomous AI Agent cluster.</p>
        </div>
        <div className="flex gap-4">
            <StatSmall icon={<MessageSquare size={14}/>} label="Platforms" value={status.dialogs?.length || 0} />
            <StatSmall icon={<Cpu size={14}/>} label="LLMs" value={status.llms?.length || 0} />
            <StatSmall icon={<Zap size={14}/>} label="Skills" value={status.skills?.length || 0} />
        </div>
      </div>

      {/* Grid Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        
        {/* Active Dialogs */}
        <Section icon={<Activity size={18}/>} title="Messaging Channels">
          <div className="divide-y border border-[var(--border-subtle)] bg-[var(--surface-card)] rounded-[var(--radius-lg)] overflow-hidden">
            {status.dialogs?.map(d => (
              <div key={d.name} className="px-6 py-4 flex items-center justify-between hover:bg-[var(--color-gray-50)] transition-colors">
                <div>
                  <div className="font-bold text-[var(--text-primary)] capitalize">{d.name}</div>
                  <div className="text-[10px] text-[var(--text-tertiary)] font-bold uppercase tracking-widest">Status: Online</div>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-[10px] font-bold text-[var(--color-success)] uppercase">Active</span>
                  <div className="w-2 h-2 rounded-full bg-[var(--color-success)] shadow-[0_0_8px_var(--color-success)]"></div>
                </div>
              </div>
            )) || <div className="text-[var(--text-tertiary)] py-8 px-6 italic text-sm text-center">No dialog platforms registered.</div>}
          </div>
        </Section>

        {/* Loaded LLMs */}
        <Section icon={<Cpu size={18}/>} title="Inference Engines">
          <div className="grid grid-cols-1 gap-4">
             {status.llms?.map(l => (
              <div key={l.name} className="card p-5 group cursor-default">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <div className="p-1.5 bg-[var(--color-accent-soft)] text-[var(--color-accent)] rounded-md">
                      <Cpu size={16} />
                    </div>
                    <div className="font-bold text-[var(--text-primary)]">{l.name}</div>
                  </div>
                  <span className="badge badge--pro !bg-[var(--color-success-soft)] !text-[var(--color-success)] !border-none">Active</span>
                </div>
                <p className="text-xs text-[var(--text-secondary)] leading-relaxed">{l.description || 'No description provided.'}</p>
              </div>
            )) || <div className="text-[var(--text-tertiary)] py-8 italic text-sm text-center">No LLM cores loaded.</div>}
          </div>
        </Section>

         {/* Recently Loaded Skills */}
         <Section icon={<HardDrive size={18}/>} title="Mastered Skills" className="lg:col-span-2">
           <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {status.skills?.map(s => (
                <div key={s.name} className="card group">
                  <div className="card-content">
                    <div className="flex items-center justify-between mb-2">
                      <div className="font-bold text-sm text-[var(--text-primary)]">{s.name}</div>
                      <div className="p-1 bg-[var(--color-gray-50)] text-[var(--text-tertiary)] rounded">
                        <Zap size={12} />
                      </div>
                    </div>
                    <p className="text-[11px] text-[var(--text-secondary)] leading-relaxed mb-4 line-clamp-2 h-8">{s.description || 'General instruction automated by Core Agent'}</p>
                    <div className="flex items-center justify-between border-t border-[var(--border-subtle)] pt-3">
                      <span className="text-[9px] font-bold text-[var(--text-tertiary)] uppercase tracking-tight">Registry: {s.manager}</span>
                      <button className="text-[9px] font-bold text-[var(--color-accent)] hover:underline uppercase tracking-tight">Configure</button>
                    </div>
                  </div>
                </div>
              ))}
              {(!status.skills || status.skills.length === 0) && (
                <div className="text-[var(--text-tertiary)] py-12 italic text-sm col-span-full text-center border border-dashed border-[var(--border-subtle)] rounded-[var(--radius-lg)]">
                  No skills discovered in active registries.
                </div>
              )}
           </div>
        </Section>
      </div>
    </div>
  );
};

const Section = ({ icon, title, children, className = "" }: { icon: React.ReactNode, title: string, children: React.ReactNode, className?: string }) => (
  <div className={`flex flex-col ${className}`}>
    <div className="flex items-center gap-3 mb-5 px-1">
      <div className="text-[var(--color-accent)]">{icon}</div>
      <h2 className="text-xs font-bold uppercase tracking-[0.2em] text-[var(--text-primary)]">{title}</h2>
      <div className="h-px flex-1 bg-[var(--border-subtle)] ml-2"></div>
    </div>
    <div className="flex-1">{children}</div>
  </div>
);

const StatSmall = ({ icon, label, value }: { icon: React.ReactNode, label: string, value: number | string }) => (
  <div className="card px-5 py-3 flex items-center gap-4 min-w-[120px] bg-[var(--surface-card)]">
    <div className="p-2 bg-[var(--color-gray-50)] text-[var(--text-tertiary)] rounded-full">
      {icon}
    </div>
    <div>
      <div className="text-[10px] font-bold text-[var(--text-tertiary)] uppercase tracking-wider mb-0.5">{label}</div>
      <div className="text-xl font-bold text-[var(--text-primary)] leading-none">{value}</div>
    </div>
  </div>
);

export default Dashboard;
