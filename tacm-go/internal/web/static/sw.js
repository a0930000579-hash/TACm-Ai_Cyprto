/* TAC Ai 智能鏈 Service Worker：核心頁面與靜態資源離線快取。 */
const CACHE = 'tacm-v4';
const CORE = ['/dashboard', '/wallet', '/exchange', '/', '/static/css/style.css', '/static/tacm.svg'];
self.addEventListener('install', function(e){
  e.waitUntil(caches.open(CACHE).then(function(c){ return c.addAll(CORE); }).then(function(){ return self.skipWaiting(); }));
});
self.addEventListener('activate', function(e){
  e.waitUntil(caches.keys().then(function(keys){
    return Promise.all(keys.filter(function(k){ return k !== CACHE; }).map(function(k){ return caches.delete(k); }));
  }).then(function(){ return self.clients.claim(); }));
});
self.addEventListener('fetch', function(e){
  var u = new URL(e.request.url);
  if (e.request.method !== 'GET' || !u.protocol.startsWith('http')) return;
  // M34：API 一律網路直連（不緩存）——避免礦機/錢包/全鏈數據「固定不動」。
  if (u.pathname.indexOf('/api/') === 0) { e.respondWith(fetch(e.request)); return; }
  // M38.6：i18n 字典一律網路直連（不緩存）——避免部署後語言版本殘留舊快取。
  if (u.pathname === '/static/js/i18n.js') {
    e.respondWith(fetch(e.request).catch(function(){ return caches.match(e.request); }));
    return;
  }
  if (u.pathname.startsWith('/static/')) {
    // 靜態資源：快取優先、背景更新（stale-while-revalidate）。
    e.respondWith(caches.match(e.request).then(function(hit){
      var net = fetch(e.request).then(function(res){
        if (res.ok) { var clone = res.clone(); caches.open(CACHE).then(function(c){ c.put(e.request, clone); }); }
        return res;
      }).catch(function(){ return hit; });
      return hit || net;
    }));
    return;
  }
  // 頁面（HTML）：網路優先，離線才用快取。
  e.respondWith(fetch(e.request).then(function(res){
    if (res.ok) { var clone = res.clone(); caches.open(CACHE).then(function(c){ c.put(e.request, clone); }); }
    return res;
  }).catch(function(){
    return caches.match(e.request).then(function(hit){ return hit || caches.match('/dashboard'); });
  }));
});
