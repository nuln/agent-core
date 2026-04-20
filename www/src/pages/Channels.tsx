import React, { useEffect, useState } from 'react';
import axios from 'axios';
import { MessageSquare, Plus, Activity, AlertCircle, CheckCircle2, QrCode, RefreshCw, Power, PowerOff, ChevronRight } from 'lucide-react';

interface ConfigField {
  key: string;
  label?: string;
  description: string;
  type: string;
  required?: boolean;
  default?: string;
  env_var?: string;
}

interface DialogInstance {
  id: string;
  status: string;
  inbound_at: number;
  description: string;
}

interface PluginInfo {
  name: string;
  category: string;
  description: string;
  auth_type?: string; // "", "qr", or "token"
  fields: ConfigField[];
  enabled: boolean;
  instances?: DialogInstance[];
}

interface AuthSession {
  id: string;
  qr_url?: string;
  status: string;
  message?: string;
}

const Channels: React.FC = () => {
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [enableTarget, setEnableTarget] = useState<PluginInfo | null>(null);
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const [enabling, setEnabling] = useState(false);
  const [authPlugin, setAuthPlugin] = useState('');
  const [authSession, setAuthSession] = useState<AuthSession | null>(null);
  const [addTarget, setAddTarget] = useState<PluginInfo | null>(null);
  const [addParams, setAddParams] = useState<Record<string, string>>({});
  const [adding, setAdding] = useState(false);

  const fetchPlugins = () => {
    axios.get('/api/v1/plugins')
      .then(res => setPlugins(res.data || []))
      .catch(err => console.error('Failed to load plugins', err))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    fetchPlugins();
    const timer = setInterval(fetchPlugins, 5000);
    return () => clearInterval(timer);
  }, []);

  const openEnableModal = (plugin: PluginInfo) => {
    if (plugin.auth_type === 'qr') {
      setEnabling(true);
      axios.post('/api/v1/plugins/enable', { name: plugin.name, config: {} })
        .then(() => { fetchPlugins(); handleStartAuth(plugin.name); })
        .catch(err => alert('Failed to enable: ' + (err.response?.data || err.message)))
        .finally(() => setEnabling(false));
      return;
    }
    const defaults: Record<string, string> = {};
    (plugin.fields || []).forEach(f => { defaults[f.key] = f.default || ''; });
    setFormValues(defaults);
    setEnableTarget(plugin);
  };

  const handleEnable = () => {
    if (!enableTarget) return;
    setEnabling(true);
    const config: Record<string, any> = {};
    (enableTarget.fields || []).forEach(f => {
      if (formValues[f.key]) config[f.key] = formValues[f.key];
    });
    axios.post('/api/v1/plugins/enable', { name: enableTarget.name, config })
      .then(() => { setEnableTarget(null); fetchPlugins(); })
      .catch(err => alert('Failed to enable: ' + (err.response?.data || err.message)))
      .finally(() => setEnabling(false));
  };

  const handleDisable = (pluginName: string) => {
    if (!confirm(`Disable ${pluginName}? All active connections will be stopped.`)) return;
    axios.post('/api/v1/plugins/disable', { name: pluginName })
      .then(() => fetchPlugins())
      .catch(err => alert('Failed to disable: ' + (err.response?.data || err.message)));
  };

  const handleStartAuth = (pluginName: string) => {
    setAuthPlugin(pluginName);
    axios.post(`/api/v1/dialogs/auth/start?platform=${pluginName}`)
      .then(res => setAuthSession(res.data));
  };

  useEffect(() => {
    if (!authSession || authSession.status === 'confirmed' || authSession.status === 'expired') return;
    const t = setInterval(() => {
      axios.get(`/api/v1/dialogs/auth/poll?platform=${authPlugin}&session_id=${authSession.id}`)
        .then(res => {
          setAuthSession(res.data);
          if (res.data.status === 'confirmed') {
            clearInterval(t);
            setTimeout(() => { setAuthSession(null); fetchPlugins(); }, 2000);
          }
        });
    }, 2000);
    return () => clearInterval(t);
  }, [authSession, authPlugin]);

  const openAddModal = (plugin: PluginInfo) => {
    const params: Record<string, string> = {};
    (plugin.fields || []).forEach(f => { params[f.key] = ''; });
    setAddParams(params);
    setAddTarget(plugin);
  };

  const handleAddAccount = () => {
    if (!addTarget) return;
    setAdding(true);
    const params: Record<string, string> = {};
    Object.entries(addParams).forEach(([k, v]) => { if (v) params[k] = v; });
    axios.post('/api/v1/dialogs/add', { platform: addTarget.name, params })
      .then(() => { setAddTarget(null); fetchPlugins(); })
      .catch(err => alert('Failed to add: ' + (err.response?.data || err.message)))
      .finally(() => setAdding(false));
  };

  if (loading && plugins.length === 0) return <div className="text-[var(--text-tertiary)] flex items-center gap-2"><Activity size={16} className="animate-spin" /> Loading integrations…</div>;

  const enabledPlugins = plugins.filter(p => p.enabled);
  const availablePlugins = plugins.filter(p => !p.enabled);

  return (
    <div className="space-y-12 animate-in fade-in duration-500">
      <div>
        <h1 className="text-4xl font-bold tracking-tight text-[var(--text-primary)]">Integrations</h1>
        <p className="text-[var(--text-secondary)] mt-2 font-medium">Enable messaging channels and manage bot accounts.</p>
      </div>

      {enabledPlugins.length > 0 && (
        <section className="space-y-6">
          <SectionHeader label="Active Integrations" />
          {enabledPlugins.map(plugin => (
            <ActivePluginCard
              key={plugin.name}
              plugin={plugin}
              onDisable={() => handleDisable(plugin.name)}
              onQrAuth={() => handleStartAuth(plugin.name)}
              onAddAccount={() => openAddModal(plugin)}
            />
          ))}
        </section>
      )}

      {availablePlugins.length > 0 && (
        <section className="space-y-6">
          <SectionHeader label="Available Integrations" />
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {availablePlugins.map(plugin => (
              <AvailablePluginCard
                key={plugin.name}
                plugin={plugin}
                onEnable={() => openEnableModal(plugin)}
              />
            ))}
          </div>
        </section>
      )}

      {plugins.length === 0 && (
        <div className="text-center py-24 border-2 border-dashed border-[var(--border-subtle)] rounded-[var(--radius-lg)]">
          <MessageSquare size={40} className="mx-auto text-[var(--color-gray-200)] mb-4" />
          <p className="text-[var(--text-tertiary)] font-medium">No integration plugins registered.</p>
        </div>
      )}

      {enableTarget && (
        <Modal title={`Enable ${capitalize(enableTarget.name)}`} subtitle="Configure and activate this integration." onClose={() => setEnableTarget(null)}>
          <div className="space-y-5">
            {(enableTarget.fields || []).length === 0 && (
              <p className="text-sm text-[var(--text-secondary)]">No configuration required.</p>
            )}
            {(enableTarget.fields || []).map(field => (
              <FieldInput
                key={field.key}
                field={field}
                value={formValues[field.key] ?? ''}
                onChange={v => setFormValues(prev => ({ ...prev, [field.key]: v }))}
              />
            ))}
            <div className="pt-4 flex gap-3">
              <button onClick={() => setEnableTarget(null)} className="button button--flat flex-1">Cancel</button>
              <button
                onClick={handleEnable}
                disabled={enabling}
                className="button flex-1"
              >
                {enabling ? 'Enabling…' : 'Enable'}
              </button>
            </div>
          </div>
        </Modal>
      )}

      {authSession && (
        <Modal title={`Add ${capitalize(authPlugin)} Account`} subtitle="Scan the QR code with your phone." onClose={() => setAuthSession(null)}>
          <div className="aspect-square bg-[var(--color-gray-50)] border border-[var(--border-subtle)] rounded-[var(--radius-md)] flex items-center justify-center relative overflow-hidden">
            {authSession.qr_url
              ? <img src={authSession.qr_url} alt="QR Code" className="w-full h-full p-6" />
              : <QrCode size={48} className="text-[var(--color-gray-200)]" />}
            {authSession.status === 'confirmed' && (
              <div className="absolute inset-0 bg-[var(--color-success)] flex flex-col items-center justify-center text-white p-6 text-center animate-in fade-in duration-300">
                <CheckCircle2 size={48} className="mb-4" />
                <div className="font-bold uppercase tracking-widest text-sm">Login Successful</div>
              </div>
            )}
            {authSession.status === 'scanned' && (
              <div className="absolute inset-x-0 bottom-0 bg-[var(--color-primary)] text-white py-3 px-4 text-center animate-in slide-in-from-bottom duration-300">
                <div className="flex items-center justify-center gap-2">
                  <Activity size={14} className="animate-pulse" />
                  <span className="text-[10px] font-bold uppercase tracking-widest">Scanned! Confirm on phone</span>
                </div>
              </div>
            )}
          </div>
          <div className="mt-6 flex items-center justify-between">
            <StatusDot status={authSession.status} />
            {authSession.status === 'expired' && (
              <button onClick={() => handleStartAuth(authPlugin)} className="text-[10px] font-bold underline uppercase text-[var(--color-error)]">Retry</button>
            )}
            <button onClick={() => setAuthSession(null)} className="button button--flat px-4 py-1.5 text-xs">Close</button>
          </div>
        </Modal>
      )}

      {addTarget && (
        <Modal title={`Add ${capitalize(addTarget.name)} Account`} subtitle="Enter your bot credentials." onClose={() => setAddTarget(null)}>
          <div className="space-y-5">
            {(addTarget.fields || []).map(field => (
              <FieldInput
                key={field.key}
                field={field}
                value={addParams[field.key] ?? ''}
                onChange={v => setAddParams(prev => ({ ...prev, [field.key]: v }))}
              />
            ))}
            <div className="pt-4 flex gap-3">
              <button onClick={() => setAddTarget(null)} className="button button--flat flex-1">Cancel</button>
              <button
                onClick={handleAddAccount}
                disabled={adding}
                className="button flex-1"
              >
                {adding ? 'Connecting…' : 'Add Account'}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
};

const SectionHeader = ({ label }: { label: string }) => (
  <div className="flex items-center gap-4">
    <div className="text-[var(--text-tertiary)] font-bold text-[10px] uppercase tracking-[0.2em]">{label}</div>
    <div className="h-px flex-1 bg-[var(--border-subtle)]" />
  </div>
);

const ActivePluginCard = ({
  plugin, onDisable, onQrAuth, onAddAccount,
}: {
  plugin: PluginInfo;
  onDisable: () => void;
  onQrAuth: () => void;
  onAddAccount: () => void;
}) => {
  const instances = plugin.instances || [];
  const supportsQr = plugin.auth_type === 'qr';

  return (
    <div className="card">
      <div className="card-title">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-[var(--color-success-soft)] text-[var(--color-success)] rounded-full">
            <Activity size={14} />
          </div>
          <div className="flex flex-col">
            <span className="font-bold text-[var(--text-primary)]">{capitalize(plugin.name)}</span>
            <span className="text-[10px] text-[var(--text-tertiary)] font-medium uppercase tracking-wider">{plugin.description}</span>
          </div>
        </div>
        <div className="flex items-center gap-3">
          {supportsQr && (
            <button onClick={onQrAuth} className="button button--flat px-3 py-1.5 text-xs">
              <QrCode size={13} /> QR Login
            </button>
          )}
          {!supportsQr && (
            <button onClick={onAddAccount} className="button button--flat px-3 py-1.5 text-xs">
              <Plus size={13} /> Add Account
            </button>
          )}
          <button onClick={onDisable} className="button button--flat px-3 py-1.5 text-xs !text-[var(--color-error)] !border-[var(--color-error-soft)] hover:!bg-[var(--color-error-soft)]">
            <PowerOff size={13} /> Disable
          </button>
        </div>
      </div>

      <div className="card-content full">
        {instances.length > 0 ? (
          <table>
            <thead>
              <tr>
                <th className="!py-2 !px-6">Account ID</th>
                <th className="!py-2 !px-6">Last Activity</th>
                <th className="!py-2 !px-6">Status</th>
              </tr>
            </thead>
            <tbody>
              {instances.map(inst => (
                <tr key={inst.id}>
                  <td>
                    <div className="flex flex-col">
                      <span className="font-bold">{inst.id}</span>
                      <span className="text-[10px] text-[var(--text-tertiary)]">{inst.description}</span>
                    </div>
                  </td>
                  <td>
                    <span className="text-[11px] text-[var(--text-secondary)] font-mono">
                      {inst.inbound_at > 0 ? new Date(inst.inbound_at * 1000).toLocaleTimeString() : 'Never'}
                    </span>
                  </td>
                  <td>
                    <StatusBadge status={inst.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="px-6 py-10 text-center text-sm text-[var(--text-tertiary)] italic">
            No accounts connected yet. {supportsQr ? 'Use QR Login to start.' : 'Click "Add Account" to connect.'}
          </div>
        )}
      </div>
    </div>
  );
};

const AvailablePluginCard = ({ plugin, onEnable }: { plugin: PluginInfo; onEnable: () => void }) => (
  <div className="card group">
    <div className="card-content flex flex-col justify-between min-h-[180px]">
      <div>
        <div className="flex items-center justify-between mb-4">
          <span className="badge badge--pro !bg-[var(--color-gray-50)] !text-[var(--text-tertiary)]">{plugin.category}</span>
          <MessageSquare size={16} className="text-[var(--color-gray-200)] group-hover:text-[var(--color-accent)] transition-colors" />
        </div>
        <div className="font-bold text-lg text-[var(--text-primary)] mb-1">{capitalize(plugin.name)}</div>
        <p className="text-xs text-[var(--text-secondary)] leading-relaxed">{plugin.description || 'Messaging integration'}</p>
      </div>
      <button
        onClick={onEnable}
        className="button mt-6 !flex !justify-between items-center group-hover:bg-[var(--color-accent)] group-hover:text-white"
      >
        <span className="flex items-center gap-2"><Power size={13} /> Enable</span>
        <ChevronRight size={13} />
      </button>
    </div>
  </div>
);

const Modal = ({ title, subtitle, onClose, children }: {
  title: string; subtitle: string; onClose: () => void; children: React.ReactNode;
}) => (
  <div className="fixed inset-0 bg-[var(--surface-overlay)] backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-in fade-in duration-200">
    <div className="card w-full max-w-md shadow-[var(--shadow-lg)] animate-in zoom-in-95 duration-200">
      <div className="card-title">
        <div>
          <h3 className="text-xl font-bold text-[var(--text-primary)]">{title}</h3>
          <p className="text-[10px] text-[var(--text-tertiary)] font-bold uppercase tracking-widest mt-0.5">{subtitle}</p>
        </div>
        <button onClick={onClose} className="p-2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors rounded-full hover:bg-[var(--color-gray-50)]">
          <Plus size={20} className="rotate-45" />
        </button>
      </div>
      <div className="card-content">
        {children}
      </div>
    </div>
  </div>
);

const FieldInput = ({ field, value, onChange }: { field: ConfigField; value: string; onChange: (v: string) => void }) => {
  const keyStr = field.key || '';
  const labelText = field.label || field.description || keyStr.replace(/_/g, ' ');
  const isSecret = field.type === 'secret' || keyStr.includes('token') || keyStr.includes('secret') || keyStr.includes('password');
  return (
    <div className="space-y-1.5">
      <label className="block text-[10px] font-bold uppercase tracking-widest text-[var(--text-tertiary)] px-1">
        {labelText}
        {field.required && <span className="ml-1 text-[var(--color-error)]">*</span>}
      </label>
      <input
        type={isSecret ? 'password' : 'text'}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={field.default ? `Default: ${field.default}` : `Enter ${keyStr}…`}
        className="input"
      />
    </div>
  );
};

const StatusBadge = ({ status }: { status: string }) => {
  const isHealthy = status === 'connected' || status === 'online';
  const isWarning = status === 'starting' || status === 'scanned';
  return (
    <div className={`badge !border-none ${isHealthy ? '!bg-[var(--color-success-soft)] !text-[var(--color-success)]' :
      isWarning ? '!bg-[var(--color-warning-soft)] !text-[var(--color-warning)]' :
        '!bg-[var(--color-error-soft)] !text-[var(--color-error)]'
      }`}>
      {isHealthy ? <CheckCircle2 size={10} className="mr-1" /> : isWarning ? <RefreshCw size={10} className="mr-1 animate-spin" /> : <AlertCircle size={10} className="mr-1" />}
      {status}
    </div>
  );
};

const StatusDot = ({ status }: { status: string }) => (
  <div className="flex items-center gap-2">
    <div className={`w-2 h-2 rounded-full ${status === 'confirmed' ? 'bg-[var(--color-success)]' : 'bg-[var(--color-warning)] animate-pulse'}`} />
    <span className="text-[10px] font-bold text-[var(--text-tertiary)] uppercase tracking-widest">Status: {status}</span>
  </div>
);

function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export default Channels;
