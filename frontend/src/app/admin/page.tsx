'use client';

import { useEffect, useState, useRef } from 'react';

let API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
let WS_BASE = process.env.NEXT_PUBLIC_WS_URL || API_BASE.replace(/^http/, 'ws');

if (typeof window !== 'undefined') {
  if (API_BASE.includes('localhost') && window.location.hostname !== 'localhost' && window.location.hostname !== '127.0.0.1') {
    API_BASE = `http://${window.location.hostname}:8080`;
    WS_BASE = `ws://${window.location.hostname}:8080`;
  }
}

// --- Types ---
interface OrderItem {
  id: string;
  order_id: string;
  catalog_item_id: string;
  item_name?: string;
  service_type?: string;
  quantity: number;
  price: number;
  attributes?: Record<string, unknown>;
}
interface Order {
  id: string;
  room_id: string;
  room_number: string;
  status: 'pending' | 'accepted' | 'completed' | 'cancelled';
  total_amount: number;
  created_at: string;
  items?: OrderItem[];
}
interface PropertyService { id: string; service_type: string; is_enabled: boolean; }
interface CatalogItem {
  id: string;
  property_id: string;
  service_type: string;
  name: string;
  description: string;
  price: number;
  is_available: boolean;
  attributes?: Record<string, unknown>;
  created_at?: string;
}

// --- Helpers ---
const badgeColors: Record<string, string> = {
  fnb: 'bg-emerald-900/50 text-emerald-400 border-emerald-500/30',
  housekeeping: 'bg-sky-900/50 text-sky-400 border-sky-500/30',
  laundry: 'bg-indigo-900/50 text-indigo-400 border-indigo-500/30',
  maintenance: 'bg-amber-900/50 text-amber-400 border-amber-500/30',
  concierge: 'bg-purple-900/50 text-purple-400 border-purple-500/30',
  default: 'bg-zinc-800 text-zinc-300 border-zinc-600',
};

export default function AdminPage() {
  const [token, setToken] = useState<string | null>(null);
  const [view, setView] = useState<'kanban' | 'config'>('kanban');

  useEffect(() => {
    // Check cookie or local storage for token
    const cookies = document.cookie.split(';');
    const authCookie = cookies.find((c) => c.trim().startsWith('admin_token='));
    if (authCookie) {
      // eslint-disable-next-line
      setToken(authCookie.split('=')[1]);
    }
  }, []);

  if (!token) {
    return <AuthView onLogin={(t) => setToken(t)} />;
  }

  return (
    <div className="bg-obsidian-950 text-zinc-100 font-sans min-h-screen flex flex-col antialiased selection:bg-gold-500 selection:text-obsidian-950">
      {/* Header */}
      <header className="w-full bg-obsidian-900 border-b border-gold-900/20 px-8 py-4 flex items-center justify-between">
        <div className="flex items-center gap-6">
          <span className="font-serif text-2xl font-bold tracking-widest text-gold-400">AURA OASIS</span>
          <div className="h-6 w-px bg-zinc-800"></div>
          <nav className="flex gap-4">
            <button
              onClick={() => setView('kanban')}
              className={`text-sm font-medium tracking-wide uppercase ${view === 'kanban' ? 'text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'}`}
            >
              Live Requests
            </button>
            <button
              onClick={() => setView('config')}
              className={`text-sm font-medium tracking-wide uppercase ${view === 'config' ? 'text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'}`}
            >
              Configuration
            </button>
          </nav>
        </div>
        <button
          onClick={() => {
            document.cookie = 'admin_token=; Max-Age=0; path=/;';
            setToken(null);
          }}
          className="text-xs bg-red-950/40 hover:bg-red-900/30 border border-red-500/30 text-red-400 font-medium px-3 py-1.5 rounded-lg transition-all"
        >
          Logout
        </button>
      </header>

      {view === 'kanban' ? <KanbanView token={token} /> : <ConfigView token={token} />}
    </div>
  );
}

// --- Auth View ---
function AuthView({ onLogin }: { onLogin: (t: string) => void }) {
  const [isLogin, setIsLogin] = useState(true);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [propName, setPropName] = useState('');
  const [error, setError] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    const endpoint = isLogin ? '/api/v1/auth/login' : '/api/v1/auth/signup';
    const payload = isLogin ? { email, password } : { email, password, property_name: propName };

    try {
      const res = await fetch(`${API_BASE}${endpoint}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Failed');
      onLogin(data.token);
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('An unknown error occurred');
      }
    }
  };

  return (
    <div className="bg-obsidian-950 min-h-screen flex items-center justify-center p-4">
      <div className="bg-obsidian-900 p-8 rounded-2xl border border-zinc-800/40 w-full max-w-md shadow-2xl">
        <h1 className="font-serif text-3xl font-bold text-gold-400 text-center mb-2">AURA OASIS</h1>
        <p className="text-zinc-400 text-center text-sm mb-8">Hospitality Operating System</p>
        
        {error && <div className="bg-red-950/40 text-red-400 text-sm p-3 rounded-lg mb-4 border border-red-900/50">{error}</div>}
        
        <form onSubmit={handleSubmit} className="space-y-4">
          {!isLogin && (
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Property Name</label>
              <input type="text" required value={propName} onChange={e => setPropName(e.target.value)} className="w-full bg-obsidian-950 border border-zinc-800 rounded-lg px-4 py-2 text-zinc-100 focus:border-gold-500 focus:outline-none" />
            </div>
          )}
          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Email</label>
            <input type="email" required value={email} onChange={e => setEmail(e.target.value)} className="w-full bg-obsidian-950 border border-zinc-800 rounded-lg px-4 py-2 text-zinc-100 focus:border-gold-500 focus:outline-none" />
          </div>
          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Password</label>
            <input type="password" required value={password} onChange={e => setPassword(e.target.value)} className="w-full bg-obsidian-950 border border-zinc-800 rounded-lg px-4 py-2 text-zinc-100 focus:border-gold-500 focus:outline-none" />
          </div>
          <button type="submit" className="w-full bg-gold-500 hover:bg-gold-600 text-obsidian-950 font-bold py-2.5 rounded-lg transition-colors mt-4">
            {isLogin ? 'Sign In to Dashboard' : 'Create Property'}
          </button>
        </form>
        
        <div className="mt-6 text-center">
          <button onClick={() => setIsLogin(!isLogin)} className="text-sm text-zinc-500 hover:text-gold-400 transition-colors">
            {isLogin ? 'New property? Register here.' : 'Already have an account? Sign in.'}
          </button>
        </div>
      </div>
    </div>
  );
}

// --- Config View ---
function ConfigView({ token }: { token: string }) {
  const [services, setServices] = useState<PropertyService[]>([]);
  const [catalog, setCatalog] = useState<CatalogItem[]>([]);
  const [loading, setLoading] = useState(true);
  
  // Editor modal/form state
  const [isEditing, setIsEditing] = useState(false);
  const [editingItem, setEditingItem] = useState<Partial<CatalogItem> | null>(null);

  useEffect(() => {
    async function fetchData() {
      try {
        const [servicesRes, catalogRes] = await Promise.all([
          fetch(`${API_BASE}/api/v1/admin/services`, {
            headers: { 'Authorization': `Bearer ${token}` }
          }),
          fetch(`${API_BASE}/api/v1/admin/catalog`, {
            headers: { 'Authorization': `Bearer ${token}` }
          })
        ]);

        if (servicesRes.ok && catalogRes.ok) {
          const srvs = await servicesRes.json();
          const cats = await catalogRes.json();
          setServices(srvs || []);
          setCatalog(cats || []);
        }
      } catch (err) {
        console.error('Error fetching admin config:', err);
      } finally {
        setLoading(false);
      }
    }
    fetchData();
  }, [token]);

  const handleToggle = async (type: string, current: boolean) => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/admin/services/toggle`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify({ service_type: type, is_enabled: !current }),
      });
      if (!res.ok) throw new Error('Toggle failed');
      setServices(services.map(s => s.service_type === type ? { ...s, is_enabled: !current } : s));
    } catch {
      alert('Error toggling service');
    }
  };

  const handleSaveItem = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingItem?.name || !editingItem.service_type) return;

    const method = editingItem.id ? 'PUT' : 'POST';
    const url = editingItem.id 
      ? `${API_BASE}/api/v1/admin/catalog/${editingItem.id}` 
      : `${API_BASE}/api/v1/admin/catalog`;

    const payload = {
      ...editingItem,
      price: Number(editingItem.price || 0),
      is_available: editingItem.is_available ?? true,
      attributes: editingItem.attributes || {}
    };

    try {
      const res = await fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify(payload),
      });

      if (!res.ok) throw new Error('Failed to save catalog item');
      
      const savedItem = await res.json();
      if (editingItem.id) {
        setCatalog(catalog.map(c => c.id === savedItem.id ? savedItem : c));
      } else {
        setCatalog([...catalog, savedItem]);
      }
      setIsEditing(false);
      setEditingItem(null);
    } catch (err: any) {
      alert(err.message || 'Error saving item');
    }
  };

  const handleDeleteItem = async (id: string) => {
    if (!confirm('Are you sure you want to delete this catalog item?')) return;
    try {
      const res = await fetch(`${API_BASE}/api/v1/admin/catalog/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (!res.ok) throw new Error('Failed to delete item');
      setCatalog(catalog.filter(c => c.id !== id));
    } catch (err: any) {
      alert(err.message || 'Error deleting item');
    }
  };

  if (loading) {
    return (
      <div className="flex-grow flex items-center justify-center p-8 text-zinc-400">
        <div className="w-8 h-8 border-2 border-gold-500 border-t-transparent rounded-full animate-spin mr-3"></div>
        Loading configuration...
      </div>
    );
  }

  return (
    <main className="flex-grow p-8 max-w-4xl mx-auto w-full pb-32">
      {/* Services List */}
      <h2 className="text-2xl font-serif text-white mb-6">Service Modules</h2>
      <div className="bg-obsidian-900 border border-zinc-800/60 rounded-2xl p-6 mb-12">
        <p className="text-zinc-400 text-sm mb-6">Enable or disable modules for your property. Changes will instantly sync to the guest QR portal.</p>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {services.map(srv => (
            <div key={srv.id} className="flex items-center justify-between p-4 bg-obsidian-950 rounded-xl border border-zinc-800/40">
              <div>
                <h3 className="text-zinc-100 font-medium capitalize">{srv.service_type}</h3>
                <p className="text-zinc-500 text-xs mt-1">Manage {srv.service_type} catalog and requests.</p>
              </div>
              <button
                onClick={() => handleToggle(srv.service_type, srv.is_enabled)}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${srv.is_enabled ? 'bg-gold-500' : 'bg-zinc-700'}`}
              >
                <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${srv.is_enabled ? 'translate-x-6' : 'translate-x-1'}`} />
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Catalog Builder */}
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-2xl font-serif text-white">Catalog Builder</h2>
        <button 
          onClick={() => {
            setEditingItem({ name: '', description: '', price: 0, service_type: 'fnb', is_available: true, attributes: {} });
            setIsEditing(true);
          }}
          className="bg-gold-500 hover:bg-gold-600 text-obsidian-950 px-4 py-2 rounded-xl text-sm font-bold transition-all duration-200 shadow-md shadow-gold-500/10"
        >
          Add New Item
        </button>
      </div>

      <div className="bg-obsidian-900 border border-zinc-800/60 rounded-2xl p-6">
        {catalog.length === 0 ? (
          <div className="text-center text-zinc-500 py-12">
            <p>No catalog items found. Click 'Add New Item' to start building your services catalog.</p>
          </div>
        ) : (
          <div className="space-y-4">
            {catalog.map(item => (
              <div key={item.id} className="flex flex-col sm:flex-row sm:items-center justify-between p-4 bg-obsidian-950 rounded-xl border border-zinc-800/40 gap-4">
                <div className="flex gap-4 items-start">
                  {item.attributes?.image_url ? (
                    <img src={item.attributes.image_url as string} alt={item.name} className="w-16 h-16 object-cover rounded-lg border border-zinc-800/40" />
                  ) : (
                    <div className="w-16 h-16 bg-zinc-900 rounded-lg flex items-center justify-center border border-zinc-800/40 text-[10px] text-zinc-500 font-bold uppercase tracking-wider">No Image</div>
                  )}
                  <div>
                    <div className="flex items-center gap-2">
                      <h3 className="text-zinc-100 font-semibold">{item.name}</h3>
                      <span className={`text-[10px] px-2 py-0.5 rounded-full border ${badgeColors[item.service_type] || badgeColors.default} uppercase tracking-wider font-bold`}>
                        {item.service_type}
                      </span>
                    </div>
                    <p className="text-zinc-400 text-xs mt-1 line-clamp-1">{item.description || 'No description provided.'}</p>
                    <div className="flex items-center gap-3 mt-1.5">
                      <span className="text-gold-400 font-mono text-xs">${item.price.toFixed(2)}</span>
                      <span className={`text-[10px] font-bold ${item.is_available ? 'text-emerald-400' : 'text-rose-400'}`}>
                        {item.is_available ? 'Available' : 'Unavailable'}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-2 self-end sm:self-center">
                  <button 
                    onClick={() => {
                      setEditingItem(item);
                      setIsEditing(true);
                    }}
                    className="bg-zinc-800 hover:bg-zinc-700 text-zinc-200 text-xs px-3.5 py-2 rounded-lg font-semibold transition-all"
                  >
                    Edit
                  </button>
                  <button 
                    onClick={() => handleDeleteItem(item.id)}
                    className="bg-rose-950/40 hover:bg-rose-900/60 text-rose-300 border border-rose-900/30 text-xs px-3.5 py-2 rounded-lg font-semibold transition-all"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Editor Modal */}
      {isEditing && editingItem && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-obsidian-900 border border-zinc-800/80 rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden animate-fade-in text-left">
            <header className="px-6 py-4 border-b border-zinc-800/60 flex justify-between items-center">
              <h3 className="text-lg font-serif text-white font-semibold">
                {editingItem.id ? 'Edit Catalog Item' : 'Add New Catalog Item'}
              </h3>
              <button 
                onClick={() => {
                  setIsEditing(false);
                  setEditingItem(null);
                }} 
                className="text-zinc-500 hover:text-zinc-300 text-lg"
              >
                &times;
              </button>
            </header>

            <form onSubmit={handleSaveItem} className="p-6 space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs text-zinc-400 uppercase tracking-wider font-bold mb-1.5">Service Type</label>
                  <select 
                    value={editingItem.service_type || 'fnb'}
                    onChange={e => setEditingItem({ ...editingItem, service_type: e.target.value })}
                    className="w-full bg-obsidian-950 border border-zinc-800 rounded-xl px-4 py-2.5 text-sm text-zinc-100 focus:outline-none focus:border-gold-500 transition"
                  >
                    <option value="fnb">Food & Beverage</option>
                    <option value="housekeeping">Housekeeping</option>
                    <option value="laundry">Laundry</option>
                    <option value="maintenance">Maintenance</option>
                    <option value="concierge">Front Desk / Concierge</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs text-zinc-400 uppercase tracking-wider font-bold mb-1.5">Price ($)</label>
                  <input 
                    type="number"
                    step="0.01"
                    min="0"
                    placeholder="0.00"
                    value={editingItem.price ?? ''}
                    onChange={e => setEditingItem({ ...editingItem, price: parseFloat(e.target.value) || 0 })}
                    className="w-full bg-obsidian-950 border border-zinc-800 rounded-xl px-4 py-2.5 text-sm text-zinc-100 font-mono focus:outline-none focus:border-gold-500 transition"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs text-zinc-400 uppercase tracking-wider font-bold mb-1.5">Item Name</label>
                <input 
                  type="text"
                  required
                  placeholder="e.g. Wagyu Beef Burger, Extra Towels, Dry Cleaning"
                  value={editingItem.name || ''}
                  onChange={e => setEditingItem({ ...editingItem, name: e.target.value })}
                  className="w-full bg-obsidian-950 border border-zinc-800 rounded-xl px-4 py-2.5 text-sm text-zinc-100 focus:outline-none focus:border-gold-500 transition"
                />
              </div>

              <div>
                <label className="block text-xs text-zinc-400 uppercase tracking-wider font-bold mb-1.5">Description</label>
                <textarea 
                  rows={3}
                  placeholder="Describe the service or item options..."
                  value={editingItem.description || ''}
                  onChange={e => setEditingItem({ ...editingItem, description: e.target.value })}
                  className="w-full bg-obsidian-950 border border-zinc-800 rounded-xl px-4 py-2.5 text-sm text-zinc-100 focus:outline-none focus:border-gold-500 transition resize-none"
                />
              </div>

              <div>
                <label className="block text-xs text-zinc-400 uppercase tracking-wider font-bold mb-1.5">Image URL</label>
                <input 
                  type="url"
                  placeholder="https://example.com/image.jpg"
                  value={(editingItem.attributes?.image_url as string) || ''}
                  onChange={e => setEditingItem({ 
                    ...editingItem, 
                    attributes: { ...editingItem.attributes, image_url: e.target.value } 
                  })}
                  className="w-full bg-obsidian-950 border border-zinc-800 rounded-xl px-4 py-2.5 text-sm text-zinc-100 focus:outline-none focus:border-gold-500 transition"
                />
              </div>

              <div className="flex flex-col sm:flex-row sm:items-center gap-4 pt-2">
                <label className="flex items-center gap-2 text-sm text-zinc-300 select-none cursor-pointer">
                  <input 
                    type="checkbox"
                    checked={editingItem.is_available ?? true}
                    onChange={e => setEditingItem({ ...editingItem, is_available: e.target.checked })}
                    className="rounded bg-obsidian-950 border-zinc-800 text-gold-500 focus:ring-0 focus:ring-offset-0"
                  />
                  Item is Available for Booking
                </label>

                {(editingItem.service_type === 'concierge' || editingItem.service_type === 'maintenance') && (
                  <label className="flex items-center gap-2 text-sm text-zinc-300 select-none cursor-pointer">
                    <input 
                      type="checkbox"
                      checked={!!editingItem.attributes?.requires_time}
                      onChange={e => setEditingItem({ 
                        ...editingItem, 
                        attributes: { ...editingItem.attributes, requires_time: e.target.checked } 
                      })}
                      className="rounded bg-obsidian-950 border-zinc-800 text-gold-500 focus:ring-0 focus:ring-offset-0"
                    />
                    Requires guest to specify a time
                  </label>
                )}
              </div>

              <footer className="flex justify-end gap-3 pt-6 border-t border-zinc-800/60 mt-6">
                <button 
                  type="button"
                  onClick={() => {
                    setIsEditing(false);
                    setEditingItem(null);
                  }}
                  className="bg-zinc-800 hover:bg-zinc-700 text-zinc-300 text-sm px-4 py-2.5 rounded-xl font-bold transition-all"
                >
                  Cancel
                </button>
                <button 
                  type="submit"
                  className="bg-gold-500 hover:bg-gold-600 text-obsidian-950 text-sm px-5 py-2.5 rounded-xl font-bold transition-all shadow-md shadow-gold-500/10"
                >
                  Save Changes
                </button>
              </footer>
            </form>
          </div>
        </div>
      )}
    </main>
  );
}

// --- Kanban View ---
function KanbanView({ token }: { token: string }) {
  const [ordersMap, setOrdersMap] = useState<{ [id: string]: Order }>({});
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    async function fetchOrders() {
      try {
        const response = await fetch(`${API_BASE}/api/v1/admin/orders`, {
          headers: { 'Authorization': `Bearer ${token}` }
        });
        if (!response.ok) throw new Error('Could not fetch historical orders');
        const orders: Order[] = await response.json();
        
        const mapped = (orders || []).reduce((acc, order) => {
          acc[order.id] = order;
          return acc;
        }, {} as { [id: string]: Order });
        
        setOrdersMap(mapped);
      } catch (err) {
        console.error('Fetch orders failed:', err);
      }
    }
    fetchOrders();
  }, [token]);

  useEffect(() => {
    function connect() {
      const wsUrl = `${WS_BASE}/ws/admin?token=${token}`; // Simplified WS Auth
      const socket = new WebSocket(wsUrl);
      wsRef.current = socket;

      socket.onopen = () => {};
      socket.onclose = () => {
        setTimeout(connect, 3000);
      };
      socket.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          if (message.type === 'order_created' || message.type === 'order_updated') {
            const updatedOrder: Order = message.payload;
            setOrdersMap((prev) => ({ ...prev, [updatedOrder.id]: updatedOrder }));
          }
        } catch {}
      };
    }
    connect();
    return () => { if (wsRef.current) wsRef.current.close(); };
  }, [token]);

  const updateStatus = async (orderId: string, newStatus: string) => {
    try {
      const response = await fetch(`${API_BASE}/api/v1/admin/orders/${orderId}/status`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify({ status: newStatus }),
      });
      if (!response.ok) throw new Error('Failed to update status');
      const updatedOrder: Order = await response.json();
      setOrdersMap((prev) => ({ ...prev, [updatedOrder.id]: updatedOrder }));
    } catch { alert('Error updating status'); }
  };

  const removeItem = async (orderId: string, itemId: string) => {
    try {
      const response = await fetch(`${API_BASE}/api/v1/admin/orders/${orderId}/items/${itemId}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (!response.ok) throw new Error('Failed to remove item');
      const updatedOrder: Order = await response.json();
      setOrdersMap((prev) => ({ ...prev, [updatedOrder.id]: updatedOrder }));
    } catch { alert('Error removing item'); }
  };

  const allOrders = Object.values(ordersMap).sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
  const pendingOrders = allOrders.filter((o) => o.status === 'pending');
  const acceptedOrders = allOrders.filter((o) => o.status === 'accepted');
  const historyOrders = allOrders.filter((o) => o.status === 'completed' || o.status === 'cancelled');

  return (
    <main className="flex-grow p-8 grid grid-cols-1 lg:grid-cols-3 gap-6 overflow-hidden">
      <KanbanColumn title="Incoming Requests" orders={pendingOrders} type="pending" onUpdateStatus={updateStatus} onRemoveItem={removeItem} />
      <KanbanColumn title="In Progress" orders={acceptedOrders} type="accepted" onUpdateStatus={updateStatus} />
      <KanbanColumn title="Fulfillment Log" orders={historyOrders} type="history" onUpdateStatus={updateStatus} />
    </main>
  );
}

// --- Kanban Components ---
interface KanbanColumnProps {
  title: string;
  orders: Order[];
  type: 'pending' | 'accepted' | 'history';
  onUpdateStatus: (orderId: string, status: string) => Promise<void>;
  onRemoveItem?: (orderId: string, itemId: string) => Promise<void>;
}

function KanbanColumn({ title, orders, type, onUpdateStatus, onRemoveItem }: KanbanColumnProps) {
  const bgClass = type === 'pending' ? 'bg-amber-500' : type === 'accepted' ? 'bg-sky-500' : 'bg-zinc-600';
  return (
    <section className="bg-obsidian-900/60 border border-zinc-800/40 rounded-2xl flex flex-col h-[calc(100vh-140px)]">
      <div className="p-5 border-b border-zinc-800/60 flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <span className={`w-3 h-3 rounded-full ${bgClass} ${type === 'pending' ? 'animate-ping' : ''}`}></span>
          <h2 className="font-serif text-lg font-bold text-zinc-100">{title}</h2>
        </div>
        <span className="bg-zinc-850 border border-zinc-700 text-zinc-400 text-xs font-bold px-2.5 py-0.5 rounded-full">{orders.length}</span>
      </div>
      <div className="p-5 flex-grow overflow-y-auto custom-scroll space-y-4">
        {orders.map((order: Order) => (
          <OrderCard key={order.id} order={order} onUpdateStatus={onUpdateStatus} onRemoveItem={onRemoveItem} />
        ))}
        {orders.length === 0 && <div className="text-center py-12 text-zinc-650 text-sm italic">Empty.</div>}
      </div>
    </section>
  );
}

interface OrderCardProps {
  order: Order;
  onUpdateStatus: (orderId: string, status: string) => Promise<void>;
  onRemoveItem?: (orderId: string, itemId: string) => Promise<void>;
}

function OrderCard({ order, onUpdateStatus, onRemoveItem }: OrderCardProps) {
  const statusBorder = order.status === 'pending' ? 'border-gold-500/40' : order.status === 'accepted' ? 'border-sky-500/30' : 'border-zinc-800/80';
  const timeStr = new Date(order.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

  return (
    <div className={`p-5 rounded-2xl bg-obsidian-900 border ${statusBorder} transition-all flex flex-col justify-between`}>
      <div>
        <div className="flex items-center justify-between mb-3">
          <span className="text-lg font-serif font-bold text-white">Room {order.room_number || '...'}</span>
          <span className="text-xs text-zinc-550 font-medium">{timeStr}</span>
        </div>
        <div className="space-y-2 my-3">
          {order.items?.length === 0 && <div className="text-xs text-red-400 italic">All items were removed.</div>}
          {order.items?.map((item: OrderItem) => {
            const badgeClass = badgeColors[item.service_type || 'default'] || badgeColors.default;
            return (
              <div key={item.id} className="flex justify-between items-start text-xs text-zinc-350 bg-obsidian-950/50 p-2 rounded-lg border border-zinc-800/50">
                <div className="flex flex-col gap-1 w-full">
                  <div className="flex justify-between items-center w-full">
                    <div className="flex items-center gap-2">
                      <span className={`text-[9px] px-1.5 py-0.5 rounded uppercase font-bold border ${badgeClass}`}>{item.service_type}</span>
                      <span className="font-medium text-zinc-200">{item.item_name} <span className="text-zinc-550">x{item.quantity}</span></span>
                    </div>
                    <div className="flex items-center gap-2">
                      {item.price > 0 && <span className="font-mono text-zinc-450">${(item.price * item.quantity).toFixed(2)}</span>}
                      {order.status === 'pending' && onRemoveItem && (
                        <button onClick={() => onRemoveItem(order.id, item.id)} className="text-red-400 hover:text-red-300 p-1">🗑️</button>
                      )}
                    </div>
                  </div>
                  {item.attributes && Object.keys(item.attributes).length > 0 && (
                    <div className="mt-1 pl-1 text-[10px] text-zinc-500 font-mono">
                      {Object.entries(item.attributes).map(([k, v]) => (
                        <div key={k}>{k}: {String(v)}</div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
      <div>
        {order.total_amount > 0 && (
          <div className="flex items-center justify-between mt-3 pt-3 border-t border-zinc-800/40 text-xs">
            <span className="text-zinc-550">Total</span>
            <span className="font-serif font-bold text-gold-400 text-sm">${order.total_amount.toFixed(2)}</span>
          </div>
        )}
        <div className="mt-4 flex justify-end gap-2">
          {order.status === 'pending' && (
            <>
              <button onClick={() => onUpdateStatus(order.id, 'cancelled')} className="bg-red-950/40 hover:bg-red-900/30 border border-red-500/30 text-red-400 text-xs font-semibold py-2 px-3 rounded-lg">Reject</button>
              <button onClick={() => onUpdateStatus(order.id, 'accepted')} className="bg-gold-500 hover:bg-gold-600 text-obsidian-950 text-xs font-semibold py-2 px-3 rounded-lg">Accept Order</button>
            </>
          )}
          {order.status === 'accepted' && (
            <button onClick={() => onUpdateStatus(order.id, 'completed')} className="bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-semibold py-2 px-3 rounded-lg">Mark Done</button>
          )}
        </div>
      </div>
    </div>
  );
}
