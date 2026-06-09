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

function OrderPageContent() {
  const searchParams = useSearchParams();
  const room = searchParams.get('room');

  const [loading, setLoading] = useState(true);
  const [splash, setSplash] = useState(true);
  const [data, setData] = useState<BootstrapResponse | null>(null);
  const [activeService, setActiveService] = useState<string | null>(null);
  
  // Cart & Orders
  const [cart, setCart] = useState<CartItem[]>([]);
  const [activeOrders, setActiveOrders] = useState<Order[]>([]);
  const [showOrderTracker, setShowOrderTracker] = useState(false);

  // Trigger state to manually refetch bootstrap configuration data
  const [refetchTrigger, setRefetchTrigger] = useState(0);

  useEffect(() => {
    if (!room) return;
    fetch(`${API_BASE}/api/v1/client/bootstrap?room=${room}`)
      .then(res => res.json())
      .then(d => {
        setData(d);
        setLoading(false);
        setTimeout(() => setSplash(false), 1000); // Snappier loading
      })
      .catch(() => setLoading(false));
  }, [room, refetchTrigger]);

  useEffect(() => {
    if (activeOrders.length === 0) return;
    const interval = setInterval(() => {
      fetch(`${API_BASE}/api/v1/client/orders?room=${room}`)
        .then((res) => res.json())
        .then((orders: Order[]) => {
          setActiveOrders(orders || []);
        })
        .catch(console.error);
    }, 4000);
    return () => clearInterval(interval);
  }, [activeOrders, room]);

  // WebSocket connection for real-time synchronization of order status, service toggling, and catalog changes
  useEffect(() => {
    if (!data?.property?.id) return;

    let socket: WebSocket | null = null;
    let reconnectTimeout: NodeJS.Timeout;

    function connect() {
      const wsUrl = `${WS_BASE}/ws/client`;
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
          if (message.type === 'order_updated' && message.payload?.room_number === room) {
            const updatedOrder = message.payload;
            console.log(`Received order update for Room ${room}:`, updatedOrder);
            setActiveOrders(prev => prev.map(o => o.id === updatedOrder.id ? updatedOrder : o));
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
  }, [data?.property?.id, room]);

  if (!room) return <div className="p-8 text-white">Error: Room parameter missing. Please scan your QR code again.</div>;
  if (loading) return <div className="min-h-screen bg-obsidian-950 flex items-center justify-center"><div className="w-8 h-8 border-2 border-gold-500 border-t-transparent rounded-full animate-spin"></div></div>;
  if (!data) return <div className="p-8 text-white">Error loading property data.</div>;

  if (splash) {
    return (
      <div className="absolute inset-0 bg-obsidian-950 z-50 flex flex-col items-center justify-center animate-fade-out" style={{ animationDelay: '0.8s' }}>
        <h1 className="text-3xl font-serif text-gold-400 mb-2 opacity-0 animate-fade-in-up">Welcome, Our Dear Guest</h1>
        <p className="text-zinc-400 text-sm opacity-0 animate-fade-in-up" style={{ animationDelay: '0.3s' }}>To {data.property.name}</p>
        <div className="mt-6 w-12 h-px bg-gold-500/50 opacity-0 animate-fade-in" style={{ animationDelay: '0.6s' }}></div>
      </div>
    );
  }

  const addToCart = (item: CatalogItem, qty: number, attrs: Record<string, unknown> = {}) => {
    setCart([...cart, { item, quantity: qty, attrs }]);
  };

  const placeOrder = async () => {
    if (cart.length === 0) return;
    const payload = {
      room_number: room,
      items: cart.map(c => ({
        catalog_item_id: c.item.id,
        quantity: c.quantity,
        attributes: c.attrs
      }))
    };
    try {
      const res = await fetch(`${API_BASE}/api/v1/orders`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        const newOrder = await res.json();
        setCart([]);
        setActiveService(null);
        setActiveOrders([...activeOrders, newOrder]);
        setShowOrderTracker(true);
      } else {
        alert('Failed to place order');
      }
    } catch {
      alert('Error placing order');
    }
  };

  // --- Dynamic Renderers ---
  const renderCatalog = (serviceType: string) => {
    const items = data.catalog.filter(i => i.service_type === serviceType);
    if (items.length === 0) return <p className="text-zinc-500 italic p-4">No services available.</p>;

    return (
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
                src={item.attributes.image_url as string} 
                alt={item.name} 
                className="w-24 h-24 object-cover rounded-lg flex-shrink-0 border border-zinc-800/40" 
              />
            )}
          </div>
        ))}
      </div>
    );
  };

  return (
    <div className="min-h-screen max-w-md mx-auto bg-obsidian-950 text-zinc-100 font-sans pb-32 selection:bg-gold-500 selection:text-obsidian-950 border-x border-zinc-900/60 shadow-2xl relative">
      <header className="sticky top-0 z-40 bg-obsidian-950/80 backdrop-blur-md border-b border-gold-900/20 px-6 py-5 flex items-center justify-between">
        <h1 className="font-serif text-xl font-bold tracking-widest text-gold-400">{data.property.name}</h1>
        <div className="bg-obsidian-900 border border-zinc-800 rounded-lg px-3 py-1.5 flex items-center gap-2">
          <span className="text-[10px] text-zinc-500 font-medium uppercase tracking-wider">Room</span>
          <span className="text-sm font-bold text-zinc-100">{room}</span>
        </div>
      </header>

      {activeOrders.length > 0 && !showOrderTracker && (
        <button onClick={() => setShowOrderTracker(true)} className="w-full bg-gold-500 text-obsidian-950 py-3 font-bold text-sm flex items-center justify-center gap-2 sticky top-20 z-30 shadow-xl shadow-gold-500/10">
          <span className="animate-pulse">●</span> View {activeOrders.length} Active Request{activeOrders.length > 1 ? 's' : ''}
        </button>
      )}

      {!activeService ? (
        <div className="p-6">
          <h2 className="text-2xl font-serif text-white mb-6">How can we assist you?</h2>
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
          <div className="px-6 py-4 flex items-center gap-4">
            <button onClick={() => setActiveService(null)} className="text-zinc-500 hover:text-gold-400 text-sm">← Back</button>
            <h2 className="text-xl font-serif text-white">{serviceTitles[activeService] || activeService}</h2>
          </div>
          {renderCatalog(activeService)}
        </div>
      )}

      {/* Cart Drawer */}
      {cart.length > 0 && (
        <div className="absolute bottom-0 left-0 w-full bg-obsidian-900 border-t border-zinc-800 p-6 z-40 rounded-t-3xl shadow-[0_-10px_40px_rgba(0,0,0,0.5)]">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-serif font-bold text-lg text-white">Current Request</h3>
            <span className="text-zinc-400 text-sm">{cart.length} items</span>
          </div>
          <button onClick={placeOrder} className="w-full bg-gold-500 hover:bg-gold-400 text-obsidian-950 font-bold py-4 rounded-xl text-lg transition-all active:scale-[0.98]">
            Submit Request
          </button>
        </div>
      )}

      {/* Tracker Overlay */}
      {showOrderTracker && (
        <div className="absolute inset-0 bg-obsidian-950/95 backdrop-blur-sm z-50 overflow-y-auto">
          <div className="p-6 pb-32">
            <div className="flex justify-between items-center mb-8 mt-4">
              <h2 className="text-2xl font-serif text-gold-400">Live Tracker</h2>
              <button onClick={() => setShowOrderTracker(false)} className="text-zinc-400 hover:text-white px-3 py-1 bg-obsidian-900 rounded-lg border border-zinc-800 text-sm">Close</button>
            </div>
            
            <div className="space-y-6">
              {activeOrders.map(order => (
                <div key={order.id} className="bg-obsidian-900 border border-zinc-800/60 rounded-2xl p-5 shadow-2xl">
                  <div className="flex justify-between items-center mb-4 pb-4 border-b border-zinc-800">
                    <span className="text-sm text-zinc-400">Order ID: <span className="font-mono">{order.id.split('-')[0]}</span></span>
                    <span className="text-xs bg-gold-500/20 text-gold-400 px-2 py-1 rounded font-bold uppercase">{order.status}</span>
                  </div>
                  <div className="space-y-2">
                    {order.items?.map((item: OrderItem) => (
                      <div key={item.id} className="flex justify-between text-sm text-zinc-300">
                        <span>{item.item_name} <span className="text-zinc-500">x{item.quantity}</span></span>
                        {item.price > 0 && <span className="font-mono">${(item.price * item.quantity).toFixed(2)}</span>}
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
              ))}
            </div>
          </div>
        </div>
      )}

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
