/* TAC 自主智能鏈 Service Worker：核心頁面與靜態資源離線快取。 */
const CACHE = 'tacm-v1';
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
  e.respondWith(
    caches.match(e.request).then(function(hit){
      return hit || fetch(e.request).then(function(res){
        if (res.ok && (u.pathname.startsWith('/static/') || u.pathname === '/' || u.pathname.startsWith('/dashboard'))) {
          var clone = res.clone();
          caches.open(CACHE).then(function(c){ c.put(e.request, clone); });
        }
        return res;
      }).catch(function(){
        return caches.match('/dashboard');
      });
    })
  );
});
