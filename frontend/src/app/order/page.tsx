'use client';

import { useEffect, useState, Suspense } from 'react';
import { useSearchParams } from 'next/navigation';

let API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
let WS_BASE = process.env.NEXT_PUBLIC_WS_URL || API_BASE.replace(/^http/, 'ws');

if (typeof window !== 'undefined') {
  if (API_BASE.includes('localhost') && window.location.hostname !== 'localhost' && window.location.hostname !== '127.0.0.1') {
    API_BASE = `http://${window.location.hostname}:8080`;
    WS_BASE = `ws://${window.location.hostname}:8080`;
  }
}

// Storage images are saved with an absolute host baked in (e.g. localhost:9000 or an old LAN IP),
// so they break for guests on other devices/networks. Rewrite the host to whatever address the
// guest actually used to reach this page -- mirrors how API_BASE follows window.location above.
// External images (e.g. Unsplash) have no :9000 port and are left untouched.
function resolveImageUrl(raw: string): string {
  if (typeof window === 'undefined') return raw;
  try {
    const u = new URL(raw);
    if (u.port === '9000') {
      u.hostname = window.location.hostname;
      return u.toString();
    }
  } catch { /* not an absolute URL; leave as-is */ }
  return raw;
}

// --- Types ---
interface CatalogItem {
  id: string;
  service_type: string;
  name: string;
  description: string;
  price: number;
  attributes?: Record<string, unknown>;
}
interface PropertyService { service_type: string; is_enabled: boolean; }
interface Property { id: string; name: string; }
interface BootstrapResponse {
  property: Property;
  services: PropertyService[];
  catalog: CatalogItem[];
  room_number: string;
}

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
  group_id?: string;
  status: 'pending' | 'accepted' | 'completed' | 'cancelled';
  total_amount: number;
  created_at: string;
  items?: OrderItem[];
}

interface CartItem {
  item: CatalogItem;
  quantity: number;
  attrs: Record<string, unknown>;
}

// --- Icons based on service type ---
const ServiceIcon = ({ type }: { type: string }) => {
  switch (type) {
    case 'fnb': return <span>🍽️</span>;
    case 'housekeeping': return <span>🛏️</span>;
    case 'laundry': return <span>👔</span>;
    case 'maintenance': return <span>🔧</span>;
    case 'concierge': return <span>🛎️</span>;
    default: return <span>✨</span>;
  }
};

const serviceTitles: Record<string, string> = {
  fnb: 'In-Room Dining',
  housekeeping: 'Housekeeping',
  laundry: 'Laundry Service',
  maintenance: 'Maintenance',
  concierge: 'Concierge & Front Desk',
};

const normalizeCategory = (cat: string | undefined | null) => {
  if (!cat) return 'General';
  const trimmed = cat.trim();
  if (!trimmed) return 'General';
  return trimmed
    .split(/\s+/)
    .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
};

function OrderPageContent() {
  const searchParams = useSearchParams();
  
  const [roomStaticToken, setRoomStaticToken] = useState<string | null>(null);
  const [resolvedSessionToken, setResolvedSessionToken] = useState<string | null>(null);
  const [isExpired, setIsExpired] = useState(false);

  const [roomNumber, setRoomNumber] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [splash, setSplash] = useState(true);
  const [data, setData] = useState<BootstrapResponse | null>(null);
  const [activeService, setActiveService] = useState<string | null>(null);
  const [activeFnbCategory, setActiveFnbCategory] = useState<string>('All');
  
  // Cart & Orders
  const [cart, setCart] = useState<CartItem[]>([]);
  const [orders, setOrders] = useState<Order[]>([]);
  const [placedOrderIds, setPlacedOrderIds] = useState<string[]>([]);
  const [showOrderTracker, setShowOrderTracker] = useState(false);
  const [trackerTab, setTrackerTab] = useState<'active' | 'history'>('active');

  // Trigger state to manually refetch bootstrap configuration data
  const [refetchTrigger, setRefetchTrigger] = useState(0);

  const [theme, setTheme] = useState<'dark' | 'light'>('dark');
  const [toasts, setToasts] = useState<{ id: string; message: string; type: 'accepted' | 'completed' | 'cancelled' }[]>([]);

  const addToast = (message: string, type: 'accepted' | 'completed' | 'cancelled') => {
    const id = Math.random().toString(36).slice(2);
    setToasts(prev => [...prev, { id, message, type }]);
    setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 4500);
  };

  useEffect(() => {
    const saved = localStorage.getItem('theme') || 'dark';
    setTheme(saved as 'dark' | 'light');
    document.documentElement.classList.toggle('light', saved === 'light');
  }, []);

  const toggleTheme = () => {
    const newTheme = theme === 'dark' ? 'light' : 'dark';
    setTheme(newTheme);
    localStorage.setItem('theme', newTheme);
    document.documentElement.classList.toggle('light', newTheme === 'light');
  };

  const [showCartDetail, setShowCartDetail] = useState(false);

  const removeFromCart = (index: number) => {
    setCart(prev => prev.filter((_, i) => i !== index));
  };

  const getGroupedOrders = (ordersList: Order[]) => {
    const groupedMap: Record<string, {
      id: string;
      group_id: string;
      status: string;
      total_amount: number;
      created_at: string;
      items: OrderItem[];
      subOrders: Order[];
    }> = {};

    const ungrouped: Order[] = [];

    (ordersList || []).forEach(order => {
      if (order.group_id) {
        if (!groupedMap[order.group_id]) {
          groupedMap[order.group_id] = {
            id: order.id,
            group_id: order.group_id,
            status: order.status,
            total_amount: 0,
            created_at: order.created_at,
            items: [],
            subOrders: []
          };
        }
        const g = groupedMap[order.group_id];
        if (order.items) {
          g.items.push(...order.items);
        }
        g.total_amount += order.total_amount;
        g.subOrders.push(order);

        // Combined status logic:
        const statuses = g.subOrders.map(so => so.status);
        if (statuses.includes('pending')) {
          g.status = 'pending';
        } else if (statuses.includes('accepted')) {
          g.status = 'accepted';
        } else if (statuses.includes('completed')) {
          g.status = 'completed';
        } else {
          g.status = 'cancelled';
        }
      } else {
        ungrouped.push(order);
      }
    });

    const groupedList = Object.values(groupedMap).map(g => ({
      id: g.id,
      group_id: g.group_id,
      status: g.status as 'pending' | 'accepted' | 'completed' | 'cancelled',
      total_amount: g.total_amount,
      created_at: g.created_at,
      items: g.items
    }));

    return [...ungrouped, ...groupedList].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
  };

  const rawActiveOrders = orders.filter(o => o.status === 'pending' || o.status === 'accepted');
  const rawPastOrders = orders.filter(o => (o.status === 'completed' || o.status === 'cancelled') && placedOrderIds.includes(o.id));

  const activeOrders = getGroupedOrders(rawActiveOrders);
  const pastOrders = getGroupedOrders(rawPastOrders);

  // 1. Resolve room static token from search params or localStorage
  useEffect(() => {
    let t = searchParams.get('token');
    if (!t && typeof window !== 'undefined') {
      t = localStorage.getItem('last_room_static_token');
    }
    if (t) {
      setRoomStaticToken(t);
      localStorage.setItem('last_room_static_token', t);
    }
  }, [searchParams]);

  // 2. Silent stay session negotiation with backend
  useEffect(() => {
    if (!roomStaticToken) return;

    const oldSessionToken = localStorage.getItem(`guest_session_${roomStaticToken}`) || '';

    fetch(`${API_BASE}/api/v1/client/session/negotiate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        room_static_token: roomStaticToken,
        old_guest_token: oldSessionToken
      })
    })
      .then(res => res.json())
      .then(d => {
        if (d.session_token) {
          localStorage.setItem(`guest_session_${roomStaticToken}`, d.session_token);
          setResolvedSessionToken(d.session_token);
        } else {
          setResolvedSessionToken(roomStaticToken);
        }
      })
      .catch(err => {
        console.error('Failed to negotiate stay session:', err);
        setResolvedSessionToken(roomStaticToken);
      });
  }, [roomStaticToken]);

  // 3. Fetch bootstrap configuration data
  useEffect(() => {
    if (!resolvedSessionToken) return;
    fetch(`${API_BASE}/api/v1/client/bootstrap?token=${resolvedSessionToken}`)
      .then(res => res.json())
      .then(d => {
        setData(d);
        if (d.room_number) {
          setRoomNumber(d.room_number);
        }
        if (d.is_expired) {
          setIsExpired(true);
          // Clear the stale archived token so the next scan starts a fresh session
          if (roomStaticToken) {
            localStorage.removeItem(`guest_session_${roomStaticToken}`);
          }
        }
        setLoading(false);
        setTimeout(() => setSplash(false), 1000);
      })
      .catch(() => setLoading(false));
  }, [resolvedSessionToken, refetchTrigger]);

  // Load placed order IDs from localStorage
  useEffect(() => {
    if (typeof window !== 'undefined') {
      const stored = localStorage.getItem('placed_order_ids');
      if (stored) {
        try {
          const parsed = JSON.parse(stored);
          setTimeout(() => {
            setPlacedOrderIds(parsed);
          }, 0);
        } catch (e) {
          console.error('Failed to parse placed_order_ids:', e);
        }
      }
    }
  }, []);

  // Initial fetch of all orders
  useEffect(() => {
    if (!resolvedSessionToken) return;
    fetch(`${API_BASE}/api/v1/client/orders?token=${resolvedSessionToken}&all=true`)
      .then((res) => res.json())
      .then((fetchedOrders: Order[]) => {
        setOrders(fetchedOrders || []);
      })
      .catch(console.error);
  }, [resolvedSessionToken]);

  // Polling interval if there are active orders
  useEffect(() => {
    if (!resolvedSessionToken) return;
    const hasActive = orders.some(o => o.status === 'pending' || o.status === 'accepted');
    if (!hasActive) return;

    const interval = setInterval(() => {
      fetch(`${API_BASE}/api/v1/client/orders?token=${resolvedSessionToken}&all=true`)
        .then((res) => res.json())
        .then((fetchedOrders: Order[]) => {
          setOrders(fetchedOrders || []);
        })
        .catch(console.error);
    }, 4000);
    return () => clearInterval(interval);
  }, [orders, resolvedSessionToken]);

  // WebSocket connection for real-time synchronization of order status, service toggling, and catalog changes
  useEffect(() => {
    if (!data?.property?.id || !resolvedSessionToken) return;

    let socket: WebSocket | null = null;
    let reconnectTimeout: NodeJS.Timeout;

    function connect() {
      const wsUrl = `${WS_BASE}/ws/client?token=${resolvedSessionToken}`;
      socket = new WebSocket(wsUrl);

      socket.onopen = () => {
        console.log('Connected to guest WS sync channel');
      };

      socket.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          
          // 1. Sync service toggles or catalog changes for this property
          if (
            (message.type === 'service_toggled' || message.type === 'catalog_updated') &&
            message.payload?.property_id === data?.property?.id
          ) {
            console.log(`Received config change (${message.type}), synchronizing state...`);
            setRefetchTrigger(prev => prev + 1);
          }

          // 2. Sync order status updates for this room in real-time
          if (message.type === 'order_updated' && message.payload?.room_number === roomNumber) {
            const updatedOrder = message.payload;
            console.log(`Received order update for Room ${roomNumber}:`, updatedOrder);
            setOrders(prev => prev.map(o => o.id === updatedOrder.id ? updatedOrder : o));

            if (updatedOrder.status === 'accepted') {
              addToast('Your request has been accepted and is being prepared', 'accepted');
            } else if (updatedOrder.status === 'completed') {
              addToast('Your request has been fulfilled', 'completed');
            } else if (updatedOrder.status === 'cancelled') {
              addToast('A request has been declined by the hotel', 'cancelled');
            }
          }

          // 3. Guest checkout — force a bootstrap refresh so the session-ended screen appears
          if (
            message.type === 'room_updated' &&
            message.payload?.action === 'guest_checked_out' &&
            message.payload?.room_number === roomNumber
          ) {
            setRefetchTrigger(prev => prev + 1);
          }
        } catch (err) {
          console.error('Failed to parse WS message:', err);
        }
      };

      socket.onclose = () => {
        console.log('Guest WS disconnected, reconnecting...');
        reconnectTimeout = setTimeout(connect, 3000);
      };
    }

    connect();

    return () => {
      if (socket) {
        socket.onclose = null;
        socket.close();
      }
      clearTimeout(reconnectTimeout);
    };
  }, [data?.property?.id, resolvedSessionToken, roomNumber]);

  if (!roomStaticToken) return <div className="p-8 text-zinc-100 text-center">Error: Secure Room token parameter missing. Please scan your Room QR code again.</div>;
  if (loading) return <div className="min-h-screen bg-obsidian-950 flex items-center justify-center"><div className="w-8 h-8 border-2 border-gold-500 border-t-transparent rounded-full animate-spin"></div></div>;
  if (!data) return <div className="p-8 text-zinc-100 text-center">Error loading property data. Please scan your Room QR code again.</div>;

  if (splash) {
    return (
      <div className="fixed inset-0 bg-obsidian-950 z-50 flex flex-col items-center justify-center animate-fade-out" style={{ animationDelay: '0.8s' }}>
        <h1 className="text-3xl font-serif text-gold-400 mb-2 opacity-0 animate-fade-in-up">Welcome, Our Dear Guest</h1>
        <p className="text-zinc-400 text-sm opacity-0 animate-fade-in-up" style={{ animationDelay: '0.3s' }}>To {data.property.name}</p>
        <div className="mt-6 w-12 h-px bg-gold-500/50 opacity-0 animate-fade-in" style={{ animationDelay: '0.6s' }}></div>
      </div>
    );
  }

  // --- Premium Read-Only Checkout Screen ---
  if (isExpired) {
    return (
      <div className="min-h-screen w-full max-w-md mx-auto bg-obsidian-950 text-zinc-100 font-sans pb-32 selection:bg-gold-500 selection:text-obsidian-950 border-x border-zinc-900/60 shadow-2xl relative flex flex-col">
        <header className="sticky top-0 z-40 bg-obsidian-950/80 backdrop-blur-md border-b border-gold-900/20 px-6 py-5 flex items-center justify-between">
          <h1 className="font-serif text-xl font-bold tracking-widest text-gold-400">{data.property.name}</h1>
          <div className="bg-obsidian-900 border border-zinc-800 rounded-lg px-3 py-1.5 flex items-center gap-2">
            <span className="text-[10px] text-zinc-500 font-medium uppercase tracking-wider">Room</span>
            <span className="text-sm font-bold text-zinc-100">{roomNumber || '...'}</span>
          </div>
        </header>

        <main className="flex-1 p-6 flex flex-col gap-6">
          <div className="bg-obsidian-900/60 border border-gold-500/20 rounded-2xl p-6 text-center shadow-lg relative overflow-hidden">
            <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-gold-600 via-gold-400 to-gold-600"></div>
            <div className="text-5xl mb-4">✨</div>
            <h2 className="text-2xl font-serif text-gold-400 font-semibold mb-2">Thank You for Staying!</h2>
            <p className="text-zinc-400 text-sm leading-relaxed">
              Your active dining and service session has ended. Below is a detailed summary of your orders and receipts from this stay.
            </p>
          </div>

          <div>
            <h3 className="text-xs font-bold uppercase tracking-widest text-zinc-500 mb-4">Order History & Receipts</h3>
            {orders.length > 0 ? (
              <div className="space-y-4">
                {orders.map(order => (
                  <div key={order.id} className="bg-obsidian-900 border border-zinc-800/60 rounded-2xl p-5 shadow-xl">
                    <div className="flex justify-between items-center mb-3 pb-3 border-b border-zinc-800">
                      <div>
                        <span className="text-xs text-zinc-500 block">Request ID</span>
                        <span className="text-sm font-mono font-semibold text-zinc-350">{order.id.split('-')[0]}</span>
                      </div>
                      <span className={`text-[10px] px-2 py-1 rounded font-bold uppercase ${
                        order.status === 'completed' 
                          ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' 
                          : order.status === 'cancelled'
                          ? 'bg-red-500/10 text-red-400 border border-red-500/20'
                          : 'bg-zinc-850 text-zinc-400 border border-zinc-800'
                      }`}>{order.status}</span>
                    </div>
                    <div className="space-y-2">
                      {order.items?.map((item: OrderItem) => (
                        <div key={item.id} className="flex justify-between text-xs text-zinc-300">
                          <span>{item.item_name} <span className="text-zinc-500">x{item.quantity}</span></span>
                          {item.price > 0 && <span className="font-mono">${(item.price * item.quantity).toFixed(2)}</span>}
                        </div>
                      ))}
                    </div>
                    {order.total_amount > 0 && (
                      <div className="mt-4 pt-4 border-t border-zinc-800 flex justify-between font-bold text-sm">
                        <span className="text-zinc-400">Total</span>
                        <span className="text-gold-400">${order.total_amount.toFixed(2)}</span>
                      </div>
                    )}
                    <div className="text-[10px] text-zinc-500 mt-3 text-right">
                      {new Date(order.created_at).toLocaleString()}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-center py-12 text-zinc-500 bg-obsidian-900/30 rounded-2xl border border-zinc-800/40">
                <p className="text-sm">No orders recorded during this stay.</p>
              </div>
            )}
          </div>
        </main>
      </div>
    );
  }

  const addToCart = (item: CatalogItem, qty: number, attrs: Record<string, unknown> = {}) => {
    setCart([...cart, { item, quantity: qty, attrs }]);
  };

  const placeOrder = async () => {
    if (cart.length === 0) return;

    // Group cart items by service_type
    const groups: Record<string, typeof cart> = {};
    cart.forEach(c => {
      const st = c.item.service_type || 'default';
      if (!groups[st]) groups[st] = [];
      groups[st].push(c);
    });

    const serviceTypes = Object.keys(groups);
    // Generate a common group_id if cart spans multiple categories
    const groupId = serviceTypes.length > 1 ? crypto.randomUUID() : '';

    try {
      const placedOrders: Order[] = [];
      for (const st of serviceTypes) {
        const payload = {
          room_token: resolvedSessionToken,
          group_id: groupId,
          items: groups[st].map(c => ({
            catalog_item_id: c.item.id,
            quantity: c.quantity,
            attributes: c.attrs
          }))
        };

        const res = await fetch(`${API_BASE}/api/v1/orders`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });

        if (!res.ok) {
          throw new Error(`Failed to place order for service module: ${st}`);
        }

        const newOrder = await res.json();
        placedOrders.push(newOrder);
      }

      setCart([]);
      setShowCartDetail(false);
      setActiveService(null);
      setOrders(prev => [...placedOrders, ...prev]);

      const newIds = placedOrders.map(o => o.id);
      const updatedIds = [...placedOrderIds, ...newIds];
      setPlacedOrderIds(updatedIds);
      if (typeof window !== 'undefined') {
        localStorage.setItem('placed_order_ids', JSON.stringify(updatedIds));
      }
      setTrackerTab('active');
      setShowOrderTracker(true);
    } catch (err: any) {
      alert(err.message || 'Error placing order');
    }
  };

  // --- Dynamic Renderers ---
  const renderCatalog = (serviceType: string) => {
    let items = data.catalog.filter(i => i.service_type === serviceType);
    if (items.length === 0) return <p className="text-zinc-500 italic p-4">No services available.</p>;

    let fnbTabs = null;
    if (serviceType === 'fnb') {
      const categories = Array.from(new Set(items.map(item => normalizeCategory(item.attributes?.category as string))));
      const allTabs = ['All', ...categories];
      const activeTab = activeFnbCategory || 'All';

      fnbTabs = (
        <div className="flex gap-2 overflow-x-auto px-6 py-2 custom-scroll scrollbar-none mb-4">
          {allTabs.map(cat => (
            <button
              key={cat}
              onClick={() => setActiveFnbCategory(cat)}
              className={`text-xs px-4 py-2 rounded-full border flex-shrink-0 transition-all font-semibold uppercase tracking-wider ${
                activeTab === cat
                  ? 'bg-gold-500 border-gold-500 text-obsidian-950 shadow-md shadow-gold-500/10'
                  : 'bg-obsidian-900 border-zinc-800 text-zinc-400 hover:text-zinc-200'
              }`}
            >
              {cat}
            </button>
          ))}
        </div>
      );

      if (activeTab !== 'All') {
        items = items.filter(item => normalizeCategory(item.attributes?.category as string) === activeTab);
      }
    }

    return (
      <div>
        {fnbTabs}
        {items.length === 0 ? (
          <p className="text-zinc-500 italic p-6 text-center text-sm">No items found in this category.</p>
        ) : (
          <div className="space-y-4 p-4">
            {items.map(item => (
              <div key={item.id} className="bg-obsidian-900 border border-zinc-800/60 p-4 rounded-xl flex gap-4 items-start shadow-md">
                <div className="flex-1 flex flex-col justify-between min-h-[96px]">
                  <div>
                    <h3 className="text-base font-serif text-zinc-100 font-semibold">{item.name}</h3>
                    <p className="text-xs text-zinc-400 mt-1 line-clamp-2">{item.description}</p>
                  </div>
                  
                  <div className="flex items-center justify-between mt-3">
                    {item.price > 0 ? (
                      <span className="text-gold-400 font-mono text-sm">${item.price.toFixed(2)}</span>
                    ) : (
                      <span className="text-emerald-400 text-[10px] font-bold uppercase tracking-wider">Complimentary</span>
                    )}
                    <button 
                      onClick={() => addToCart(item, 1, item.attributes?.requires_time ? { requested_time: '08:00 AM' } : {})}
                      className="bg-zinc-800 hover:bg-gold-500 hover:text-obsidian-950 text-zinc-300 text-[10px] px-3 py-1.5 rounded-md font-bold transition-all duration-200"
                    >
                      Add
                    </button>
                  </div>
                </div>

                {!!item.attributes?.image_url && (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={resolveImageUrl(item.attributes.image_url as string)}
                    alt={item.name}
                    className="w-24 h-24 object-cover rounded-lg flex-shrink-0 border border-zinc-800/40" 
                  />
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="min-h-screen w-full max-w-md mx-auto bg-obsidian-950 text-zinc-100 font-sans pb-32 selection:bg-gold-500 selection:text-obsidian-950 border-x border-zinc-900/60 shadow-2xl relative">
      <header className="sticky top-0 z-40 bg-obsidian-950/80 backdrop-blur-md border-b border-gold-900/20 px-6 py-5 flex items-center justify-between">
        <h1 className="font-serif text-xl font-bold tracking-widest text-gold-400">{data.property.name}</h1>
        <div className="flex items-center gap-3">
          <button
            onClick={toggleTheme}
            className="bg-obsidian-900 border border-zinc-800 hover:border-gold-500/50 rounded-lg p-2.5 flex items-center justify-center text-zinc-300 hover:text-gold-400 transition-all active:scale-95"
            title="Toggle Theme"
          >
            <span className="text-base">{theme === 'dark' ? '☀️' : '🌙'}</span>
          </button>
          <button
            onClick={() => { setShowOrderTracker(true); setTrackerTab('history'); }}
            className="bg-obsidian-900 border border-zinc-800 hover:border-gold-500/50 rounded-lg p-2.5 flex items-center justify-center text-zinc-300 hover:text-gold-400 transition-all active:scale-95 relative"
            title="Request History"
          >
            <span className="text-base">📜</span>
            {pastOrders.length > 0 && (
              <span className="absolute -top-1 -right-1 bg-gold-500 text-obsidian-950 text-[9px] font-bold w-4 h-4 rounded-full flex items-center justify-center border border-obsidian-950 animate-fade-in">
                {pastOrders.length}
              </span>
            )}
          </button>
          <div className="bg-obsidian-900 border border-zinc-800 rounded-lg px-3 py-1.5 flex items-center gap-2">
            <span className="text-[10px] text-zinc-500 font-medium uppercase tracking-wider">Room</span>
            <span className="text-sm font-bold text-zinc-100">{roomNumber || '...'}</span>
          </div>
        </div>
      </header>

      {activeOrders.length > 0 && !showOrderTracker && (
        <button onClick={() => { setShowOrderTracker(true); setTrackerTab('active'); }} className="w-full bg-gold-500 text-obsidian-950 py-3 font-bold text-sm flex items-center justify-center gap-2 sticky top-20 z-30 shadow-xl shadow-gold-500/10">
          <span className="animate-pulse">●</span> View {activeOrders.length} Active Request{activeOrders.length > 1 ? 's' : ''}
        </button>
      )}

      {!activeService ? (
        <div className="p-6">
          <h2 className="text-2xl font-serif text-zinc-100 mb-6">How can we assist you?</h2>
          <div className="grid grid-cols-2 gap-4">
            {data.services.filter(s => s.is_enabled).map(srv => (
              <button
                key={srv.service_type}
                onClick={() => setActiveService(srv.service_type)}
                className="bg-obsidian-900 border border-zinc-800 hover:border-gold-500/50 rounded-2xl p-6 flex flex-col items-center justify-center text-center gap-3 transition-all active:scale-95 shadow-lg shadow-black/50 aspect-square"
              >
                <div className="text-4xl"><ServiceIcon type={srv.service_type} /></div>
                <span className="font-serif font-medium text-sm text-zinc-300">{serviceTitles[srv.service_type] || srv.service_type}</span>
              </button>
            ))}
          </div>
        </div>
      ) : (
        <div>
          <div className="px-6 pt-6 pb-2">
            <button 
              onClick={() => { setActiveService(null); setActiveFnbCategory('All'); }} 
              className="flex items-center gap-1.5 text-zinc-500 hover:text-gold-400 transition-colors text-xs uppercase tracking-widest font-bold mb-3"
            >
              ← Back to Services
            </button>
            <h2 className="text-2xl font-serif text-zinc-100 font-semibold">{serviceTitles[activeService] || activeService}</h2>
          </div>
          {renderCatalog(activeService)}
        </div>
      )}

      {/* Cart Drawer Bar */}
      {cart.length > 0 && !showCartDetail && (
        <div 
          onClick={() => setShowCartDetail(true)}
          className="fixed bottom-0 left-1/2 -translate-x-1/2 w-full max-w-md bg-obsidian-900 border-t border-zinc-850 p-5 z-40 rounded-t-3xl shadow-[0_-10px_40px_rgba(0,0,0,0.5)] cursor-pointer hover:bg-obsidian-850 transition-all flex items-center justify-between"
        >
          <div>
            <h3 className="font-serif font-bold text-md text-zinc-100">Review Request</h3>
            <p className="text-zinc-400 text-xs mt-0.5">{cart.length} item{cart.length > 1 ? 's' : ''} selected</p>
          </div>
          <button className="bg-gold-500 hover:bg-gold-400 text-obsidian-950 font-bold py-2 px-5 rounded-xl text-xs transition-all cursor-pointer">
            Review Details
          </button>
        </div>
      )}

      {/* Cart Detail Modal */}
      {showCartDetail && cart.length > 0 && (
        <div className="fixed inset-0 bg-obsidian-950/95 backdrop-blur-sm z-50 overflow-y-auto">
          <div className="max-w-md mx-auto p-6 pb-32">
            <div className="flex justify-between items-center mb-8 mt-4">
              <h2 className="text-2xl font-serif text-gold-400">Review Your Request</h2>
              <button onClick={() => setShowCartDetail(false)} className="text-zinc-400 hover:text-zinc-100 px-3 py-1 bg-obsidian-900 rounded-lg border border-zinc-800 text-sm cursor-pointer">Close</button>
            </div>

            <div className="space-y-4">
              {cart.map((cartItem, index) => (
                <div key={index} className="bg-obsidian-900 border border-zinc-800/60 rounded-2xl p-4 flex justify-between items-center shadow-lg">
                  <div>
                    <h4 className="font-semibold text-zinc-100 text-sm">{cartItem.item.name}</h4>
                    <p className="text-[10px] text-gold-400/80 font-bold uppercase tracking-wider mt-0.5">{serviceTitles[cartItem.item.service_type] || cartItem.item.service_type}</p>
                    <div className="text-xs text-zinc-400 mt-1 flex items-center gap-2">
                      <span>Quantity: <span className="font-mono text-zinc-200">{cartItem.quantity}</span></span>
                      {cartItem.item.price > 0
                        ? <span className="font-mono text-zinc-300">${(cartItem.item.price * cartItem.quantity).toFixed(2)}</span>
                        : <span className="text-emerald-400 text-[10px] font-bold uppercase tracking-wider">Complimentary</span>
                      }
                    </div>
                  </div>
                  <button 
                    onClick={() => removeFromCart(index)}
                    className="text-red-400 hover:text-red-300 p-2 bg-red-950/30 hover:bg-red-900/30 border border-red-500/20 rounded-xl transition-all cursor-pointer text-xs"
                    title="Remove Item"
                  >
                    🗑️
                  </button>
                </div>
              ))}
            </div>

            {/* Total price check */}
            {cart.reduce((sum, item) => sum + item.item.price * item.quantity, 0) > 0 && (
              <div className="mt-6 bg-obsidian-900/60 border border-zinc-800/60 rounded-2xl p-4 flex justify-between items-center">
                <span className="text-sm font-semibold text-zinc-400">Estimated Total</span>
                <span className="text-lg font-bold text-gold-400">${cart.reduce((sum, item) => sum + item.item.price * item.quantity, 0).toFixed(2)}</span>
              </div>
            )}

            <div className="mt-8 space-y-3">
              <button 
                onClick={placeOrder} 
                className="w-full bg-gold-500 hover:bg-gold-400 text-obsidian-950 font-bold py-4 rounded-xl text-lg transition-all active:scale-[0.98] cursor-pointer"
              >
                Submit Request
              </button>
              <button 
                onClick={() => { setCart([]); setShowCartDetail(false); }} 
                className="w-full bg-red-950/20 hover:bg-red-900/20 border border-red-500/30 text-red-400 font-semibold py-3 rounded-xl text-sm transition-all cursor-pointer"
              >
                Cancel & Clear Cart
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Tracker Overlay */}
      {showOrderTracker && (
        <div className="fixed inset-0 bg-obsidian-950/95 backdrop-blur-sm z-50 overflow-y-auto">
          <div className="max-w-md mx-auto p-6 pb-32">
            <div className="flex justify-between items-center mb-8 mt-4">
              <h2 className="text-2xl font-serif text-gold-400">My Requests</h2>
              <button onClick={() => setShowOrderTracker(false)} className="text-zinc-400 hover:text-zinc-100 px-3 py-1 bg-obsidian-900 rounded-lg border border-zinc-800 text-sm">Close</button>
            </div>

            {/* Tab Switcher */}
            <div className="flex gap-2 mb-6 p-1 bg-obsidian-900 border border-zinc-800/60 rounded-xl">
              <button
                onClick={() => setTrackerTab('active')}
                className={`flex-1 py-2.5 text-xs font-semibold rounded-lg transition-all ${trackerTab === 'active' ? 'bg-gold-500 text-obsidian-950 shadow-md font-bold' : 'text-zinc-400 hover:text-zinc-200'}`}
              >
                Active ({activeOrders.length})
              </button>
              <button
                onClick={() => setTrackerTab('history')}
                className={`flex-1 py-2.5 text-xs font-semibold rounded-lg transition-all ${trackerTab === 'history' ? 'bg-gold-500 text-obsidian-950 shadow-md font-bold' : 'text-zinc-400 hover:text-zinc-200'}`}
              >
                History ({pastOrders.length})
              </button>
            </div>
            
            <div className="space-y-6">
              {trackerTab === 'active' && activeOrders.length > 1 && activeOrders.reduce((s, o) => s + o.total_amount, 0) > 0 && (
                <div className="bg-obsidian-900 border border-gold-500/20 rounded-2xl px-5 py-4 flex justify-between items-center">
                  <div>
                    <span className="text-xs text-zinc-500 uppercase tracking-widest font-bold">Grand Total</span>
                    <p className="text-[10px] text-zinc-600 mt-0.5">{activeOrders.length} active requests</p>
                  </div>
                  <span className="text-xl font-bold font-serif text-gold-400">
                    ${activeOrders.reduce((s, o) => s + o.total_amount, 0).toFixed(2)}
                  </span>
                </div>
              )}
              {trackerTab === 'active' ? (
                activeOrders.length > 0 ? (
                  activeOrders.map(order => (
                    <div key={order.id} className="bg-obsidian-900 border border-zinc-800/60 rounded-2xl p-5 shadow-2xl">
                      <div className="flex justify-between items-center mb-4 pb-4 border-b border-zinc-800">
                        <span className="text-sm text-zinc-400">Request ID: <span className="font-mono">{order.id.split('-')[0]}</span></span>
                        <span className="text-xs bg-gold-500/20 text-gold-400 px-2 py-1 rounded font-bold uppercase">{order.status}</span>
                      </div>
                      <div className="space-y-2">
                        {order.items?.map((item: OrderItem) => (
                          <div key={item.id} className="flex justify-between text-sm text-zinc-300">
                            <span>{item.item_name} <span className="text-zinc-555">x{item.quantity}</span></span>
                            {item.price > 0
                              ? <span className="font-mono">${(item.price * item.quantity).toFixed(2)}</span>
                              : <span className="text-emerald-400 text-[10px] font-bold uppercase tracking-wider self-center">Complimentary</span>
                            }
                          </div>
                        ))}
                      </div>
                      {order.total_amount > 0 && (
                        <div className="mt-4 pt-4 border-t border-zinc-800 flex justify-between font-bold">
                          <span className="text-zinc-400">Total</span>
                          <span className="text-gold-400">${order.total_amount.toFixed(2)}</span>
                        </div>
                      )}
                    </div>
                  ))
                ) : (
                  <div className="text-center py-16 text-zinc-500 bg-obsidian-900/40 rounded-2xl border border-zinc-800/40">
                    <div className="text-4xl mb-3">🛎️</div>
                    <p className="font-serif text-sm">No active requests</p>
                    <p className="text-xs text-zinc-600 mt-1">Submit assistance/dining requests from the menu</p>
                  </div>
                )
              ) : (
                pastOrders.length > 0 ? (
                  pastOrders.map(order => (
                    <div key={order.id} className="bg-obsidian-900/60 border border-zinc-800/40 rounded-2xl p-5 shadow-xl opacity-90">
                      <div className="flex justify-between items-center mb-4 pb-4 border-b border-zinc-850">
                        <span className="text-sm text-zinc-400">Request ID: <span className="font-mono">{order.id.split('-')[0]}</span></span>
                        <span className={`text-xs px-2 py-1 rounded font-bold uppercase ${
                          order.status === 'completed' 
                            ? 'bg-emerald-500/10 text-emerald-400' 
                            : 'bg-zinc-800 text-zinc-400'
                        }`}>{order.status}</span>
                      </div>
                      <div className="space-y-2">
                        {order.items?.map((item: OrderItem) => (
                          <div key={item.id} className="flex justify-between text-sm text-zinc-400">
                            <span>{item.item_name} <span className="text-zinc-555">x{item.quantity}</span></span>
                            {item.price > 0
                              ? <span className="font-mono">${(item.price * item.quantity).toFixed(2)}</span>
                              : <span className="text-emerald-400/70 text-[10px] font-bold uppercase tracking-wider self-center">Complimentary</span>
                            }
                          </div>
                        ))}
                      </div>
                      {order.total_amount > 0 && (
                        <div className="mt-4 pt-4 border-t border-zinc-850 flex justify-between font-medium">
                          <span className="text-zinc-555 text-sm">Total</span>
                          <span className="text-gold-400/80 font-mono text-sm">${order.total_amount.toFixed(2)}</span>
                        </div>
                      )}
                      <div className="text-[10px] text-zinc-555 mt-3 text-right">
                        {new Date(order.created_at).toLocaleString()}
                      </div>
                    </div>
                  ))
                ) : (
                  <div className="text-center py-16 text-zinc-500 bg-obsidian-900/40 rounded-2xl border border-zinc-800/40">
                    <div className="text-4xl mb-3">📜</div>
                    <p className="font-serif text-sm">No request history yet</p>
                    <p className="text-xs text-zinc-600 mt-1">Completed and cancelled requests will appear here</p>
                  </div>
                )
              )}
            </div>
          </div>
        </div>
      )}

      {/* Toast Notifications */}
      <div className="fixed bottom-28 left-1/2 -translate-x-1/2 w-[90%] max-w-sm z-[60] flex flex-col gap-2 pointer-events-none">
        {toasts.map(toast => (
          <div
            key={toast.id}
            className={`px-4 py-3 rounded-xl text-sm font-semibold shadow-2xl animate-fade-in-up flex items-center gap-2 ${
              toast.type === 'accepted' ? 'bg-gold-500 text-obsidian-950' :
              toast.type === 'completed' ? 'bg-emerald-500 text-white' :
              'bg-red-500 text-white'
            }`}
          >
            <span>{toast.type === 'accepted' ? '✅' : toast.type === 'completed' ? '🎉' : '❌'}</span>
            <span>{toast.message}</span>
          </div>
        ))}
      </div>

      {/* CSS Animations */}
      <style dangerouslySetInnerHTML={{__html: `
        @keyframes fadeOut { from { opacity: 1; } to { opacity: 0; visibility: hidden; } }
        @keyframes fadeInUp { from { opacity: 0; transform: translateY(20px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
        .animate-fade-out { animation: fadeOut 0.5s ease-out forwards; }
        .animate-fade-in-up { animation: fadeInUp 0.8s ease-out forwards; }
        .animate-fade-in { animation: fadeIn 0.8s ease-out forwards; }
      `}} />
    </div>
  );
}

export default function OrderPage() {
  return (
    <Suspense fallback={<div className="min-h-screen bg-obsidian-950" />}>
      <OrderPageContent />
    </Suspense>
  );
}
