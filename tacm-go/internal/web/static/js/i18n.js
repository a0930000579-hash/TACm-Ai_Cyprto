// M39：TAC 鏈全套中英介面——預設英文，中文為第二語言（可切換）。
// 機制：data-i18n 精確替換 + TreeWalker 全域文字節點替換（雙向字典）+ TAC_T() 供 JS 動態字串使用。
(function () {
  'use strict';
  var dict = {
    zh: {
      'nav.home': '首頁', 'nav.wallet': '錢包', 'nav.exchange': '交易所', 'nav.dash': '儀表板',
      'nav.mining': '挖礦', 'nav.community': '社群',
      'pill.standby': '待機', 'pill.mining': '挖礦中', 'pill.login': '登入/註冊',
      'pill.tools': '☰ 工具', 'pill.wp': '白皮書', 'pill.defi': 'DeFi', 'pill.c2c': 'C2C', 'pill.en': 'EN', 'pill.lang': '中',
      'mining.start': '⚡ 開機挖礦', 'mining.stop': '停止挖礦', 'mining.online': '在線挖礦中', 'mining.offline': '離線',
      'wallet.title': 'TAC 錢包', 'wallet.ledger': '最近帳本流（審計分錄）', 'wallet.transfer': '站內轉帳', 'wallet.confirm': '確認轉帳',
      'dash.title': '儀表板', 'chain.mined': '全鏈總產出 TACm', 'chain.tiusd': 'TiUSD 總流通',
      'chain.pool': '獎勵池', 'chain.earned': '我的累計收益', 'chain.rate': '出塊速率',
      'index.title': 'TAC 自主智能鏈', 'index.overview': '全鏈儀表',
      'auth.register': '註冊', 'auth.login': '登入', 'auth.email': 'Email', 'auth.password': '密碼',
      'common.copy': '複製', 'common.refresh': '刷新', 'common.close': '關閉', 'common.confirm': '確認'
    },
    en: {
      'nav.home': 'Home', 'nav.wallet': 'Wallet', 'nav.exchange': 'Exchange', 'nav.dash': 'Dashboard',
      'nav.mining': 'Mining', 'nav.community': 'Community',
      'pill.standby': 'Idle', 'pill.mining': 'Mining', 'pill.login': 'Sign In / Sign Up',
      'pill.tools': '☰ Tools', 'pill.wp': 'Whitepaper', 'pill.defi': 'DeFi', 'pill.c2c': 'C2C', 'pill.en': 'EN', 'pill.lang': '中',
      'mining.start': '⚡ Start Mining', 'mining.stop': 'Stop Mining', 'mining.online': 'Mining Online', 'mining.offline': 'Offline',
      'wallet.title': 'TAC Wallet', 'wallet.ledger': 'Recent Ledger (Audit)', 'wallet.transfer': 'Internal Transfer', 'wallet.confirm': 'Confirm Transfer',
      'dash.title': 'Dashboard', 'chain.mined': 'Total TACm Mined', 'chain.tiusd': 'TiUSD Circulation',
      'chain.pool': 'Reward Pool', 'chain.earned': 'My Earnings', 'chain.rate': 'Block Rate',
      'index.title': 'TAC Autonomous Chain', 'index.overview': 'Chain Overview',
      'auth.register': 'Register', 'auth.login': 'Log In', 'auth.email': 'Email', 'auth.password': 'Password',
      'common.copy': 'Copy', 'common.refresh': 'Refresh', 'common.close': 'Close', 'common.confirm': 'Confirm'
    }
  };
  // 全域雙向字典（長短語優先替換，覆蓋所有頁面靜態文字）。
  var zh2en = {
    '區塊瀏覽器': 'Block Explorer', '首頁': 'Home', '錢包': 'Wallet', '交易所': 'Exchange',
    '儀表板': 'Dashboard', '挖礦': 'Mining', '社群': 'Community', '白皮書': 'Whitepaper',
    '待機': 'Idle', '挖礦中': 'Mining', '登入/註冊': 'Sign In / Sign Up', '登入': 'Sign In', '註冊': 'Sign Up',
    '工具': 'Tools', '登出': 'Log Out', '未登入': 'Not Signed In', '我的錢包': 'My Wallet', '我的礦機': 'My Miner',
    '登入註冊': 'Sign In / Sign Up', '官方網站': 'Official Site', '官網': 'Official Site',
    '社群陸續開放': 'Community Opening Soon', '更多': 'More',
    '鏈頂高度': 'Chain Height', '最終化高度': 'Finalized Height', '當前難度': 'Difficulty', '待打包交易': 'Pending Txs',
    '全鏈總產出': 'Total Mined', '全鏈總產出 TACm': 'Total TACm Mined', 'TiUSD 總流通': 'TiUSD Circulation',
    '獎勵池': 'Reward Pool', '出塊速率': 'Block Rate', '我的累計收益': 'My Earnings',
    '全鏈儀表': 'Chain Overview', '最新區塊列表': 'Latest Blocks', '區塊列表': 'Blocks',
    '區塊哈希': 'Block Hash', '交易數': 'Txs', '提議者': 'Proposer', '時間': 'Time', '高度': 'Height',
    '區塊': 'Block', '區塊瀏覽': 'Block Explorer', '交易哈希': 'Tx Hash', '交易': 'Tx',
    '發送方': 'Sender', '接收方': 'Receiver', '金額': 'Amount', '手續費': 'Fee', '備註': 'Memo', '狀態': 'Status',
    '載入中': 'Loading', '失敗': 'Failed', '成功': 'Success', '已確認': 'Confirmed', '待確認': 'Pending',
    '在線': 'Online', '離線': 'Offline', '在線挖礦中': 'Mining Online', '開機挖礦': 'Start Mining', '開機': 'Start',
    '停止挖礦': 'Stop Mining', '停止': 'Stop', '算力': 'Hashrate', '算力佔比': 'Hashrate Share', '總算力': 'Total Hashrate',
    '全鏈總覽': 'Chain Overview', '全網算力': 'Network Hashrate', '我的算力': 'My Hashrate',
    '維度': 'Difficulty', '礦工排行': 'Miner Ranking', '排行': 'Ranking', '平台': 'Platform',
    '台': 'unit(s)', '目前無在線礦工': 'No miners online', '無在線礦工': 'No miners online',
    '在線礦工': 'Online Miners', '礦工': 'Miner', '線上': 'Online',
    '自主智能鏈': 'Autonomous Chain', '穩定幣': 'Stablecoin', '原生幣': 'Native Coin',
    '分鐘前': 'min ago', '小時前': 'h ago', '天前': 'd ago', '剛剛': 'just now',
    '返回': 'Back', '鏈頂': 'Height', '難度': 'Difficulty', '私鑰': 'Private Key',
    '綠燈': 'Active', '灰燈': 'Idle', '綠點': 'Online', '邀請碼': 'Invite Code', '每邀請': 'Per Invite',
    '效率圓環': 'Efficiency Ring', '圓環': 'Ring', '訂單': 'Order', '餘額': 'Balance',
    '費用': 'Fee', '兌換': 'Swap', '快速兌換': 'Quick Swap', '限價': 'Limit', '市價': 'Market',
    '掛單': 'Order', '撤單': 'Cancel', '撮合': 'Match', '點差': 'Spread', '頁籤': 'Tab',
    '運行中': 'Running', '已啟動': 'Started', '已撤單': 'Cancelled', '處理中': 'Processing', '投放中': 'Active',
    '說明': 'Info', '機制': 'Mechanism', '來源': 'Source', '操作': 'Action', '用途': 'Purpose',
    '費率': 'Rate', '取款': 'Withdraw', '領取': 'Claim', '移除': 'Remove', '抵押': 'Collateral',
    '還款': 'Repay', '贖回': 'Redeem', '剩餘': 'Remaining', '下單': 'Place Order',
    '獲得': 'Earn', '最多': 'Max', '帳戶': 'Account', '輸入': 'Enter', '貼上': 'Paste',
    '無效': 'Invalid', '產生': 'Generate', '送出中': 'Sending', '送出': 'Send',
    '合計': 'Total', '實付': 'Paid', '發文': 'Post', '想法': 'Idea', '商品': 'Item',
    '轉入': 'Transfer In', '收幣': 'Receive Coins', '法幣': 'Fiat', '再查': 'Retry', '展開': 'Expand',
    '支持': 'Support', '我的': 'My', '切換': 'Switch', '完整': 'Full', '發行': 'Issuance',
    '年息': 'Annual Interest', '分潤': 'Profit Share', '挹注比例': 'Contribution Ratio',
    '記憶體池': 'Mempool', '已最終化': 'Finalized', '無法連接': 'Cannot Connect',
    '清算價': 'Liquidation Price', '已結清': 'Settled', '應計利息': 'Accrued Interest',
    '尚無貸款': 'No Loans', '項目發行': 'Project Launch', '最大可借': 'Max Borrow',
    '尚無分錄': 'No Entries', '網路錯誤': 'Network Error', '估值對映': 'Valuation',
    '鏈上機制': 'On-chain', '請先貼上': 'Paste First', '移除重複': 'Remove Duplicates',
    '換裝置時用下方': 'Use below when changing devices', '鏈上轉帳': 'On-chain Transfer',
    '一鍵轉帳': 'One-click Transfer', '付款投放': 'Paid Promotion', '來發第一篇吧': 'Be the first to post',
    '內容不可為空': 'Content cannot be empty', '直連不緩存': 'Direct No-cache',
    '請稍後再試': 'Try Again Later', '請轉至': 'Go to', '對手方': 'Counterparty',
    '創世': 'Genesis', '發出': 'Sent', '收入': 'Received', '至少': 'At least',
    '託管': 'Escrow', '轉帳費由託管帳戶承擔': 'Transfer fee paid by escrow',
    '含鏈上轉帳費': 'Includes on-chain fee', '池內可用資金': 'Pool available funds',
    '流動性用途': 'Liquidity use', '報價更優惠': 'Better quotes', '市價可留空': 'Market price optional',
    '出塊與獎勵機制': 'Block & Reward Mechanism', '原本方式': 'original mode',
    '個人資產': 'personal assets', '僅顯示': 'shows only', '總覽': 'Overview',
    '資產說明': 'Assets Info', '錢包同步區塊': 'Wallet synced to Block', '區塊瀏覽器': 'Block Explorer',
    '主網': 'Mainnet', '主網資產': 'Mainnet Assets', '收款地址': 'Recipient Address', '付款地址': 'Payment Address',
    '同步': 'Sync', '估值': 'Valuation', '自動鑄造': 'auto-mint', '鏈上機制': 'on-chain mechanism',
    '供給層': 'supply-layer', '結算': 'settlement', '入帳': 'credit', '禁止': 'prohibited',
    '發行或銷毀': 'issuance or burning', '私自': 'private', '銷毀': 'burn', '最近': 'Recent', '尚無': 'No ', '塊': ' blocks', '每出塊': 'Per Block',
    '最近成交': 'Recent Trades', '區塊交易': 'Block Tx', '我的算力佔比': 'My Hashrate Share',
    '最終化': 'Finalized', '上一區塊': 'Prev Block', '上一': 'Prev ', 'Merkle根': 'Merkle Root', 'Merkle 根': 'Merkle Root',
    '地址': 'Address', '節點': 'Node', '帳號': 'Account', '密碼': 'Password', 'Email': 'Email',
    '請輸入帳號': 'Please enter account', '請填寫帳號與金額': 'Please fill in account and amount',
    '請填寫帳號與數量': 'Please fill in account and quantity', '請輸入密碼': 'Please enter password',
    '至少 6 碼': 'At least 6 characters', '請選擇資產並輸入金額': 'Select asset and enter amount',
    '確認轉帳': 'Confirm Transfer', '站內轉帳': 'Internal Transfer', '轉帳手續費': 'Transfer Fee',
    '鏈上費': 'On-chain Fee', '鏈上手續費': 'On-chain Fee', '可用餘額': 'Available Balance', '總資產': 'Total Assets',
    '訪客': 'Guest', '訪客獨立地址': 'Guest Address', '訪客私鑰備份': 'Guest Key Backup',
    '訪客不再共用節點地址': 'Guests no longer share node address', '請妥善保管': 'Keep it safe',
    '首次由伺服器生成': 'First generated by server', '恢復': 'Restore', '備份': 'Backup', '選填': 'Optional',
    '付款地址': 'Payment Address', '複製': 'Copy', '刷新': 'Refresh', '關閉': 'Close', '確認': 'Confirm',
    '資產': 'Assets', '資產餘額': 'Asset Balance', '總流通': 'Circulation', '流通': 'Circulation',
    '最近帳本流': 'Recent Ledger', '審計分錄': 'Audit Entries', '變動': 'Change', '類型': 'Type',
    '數量': 'Amount', '預算': 'Budget', '價值': 'Value', '價格': 'Price', '方向': 'Side',
    '市場': 'Market', '買入': 'Buy', '賣出': 'Sell', '購買': 'Buy', '出售': 'Sell', '查詢': 'Search',
    '選擇': 'Select', '選擇交易對': 'Select Pair', '選擇資產': 'Select Asset', '選擇幣種': 'Select Coin',
    '交易對': 'Pair', '最新': 'Latest', '手續費(taker)': 'Fee (taker)', '委託': 'Order', '我的訂單': 'My Orders',
    '我的廣告': 'My Ads', '發佈廣告': 'Post Ad', '發佈失敗': 'Post Failed', '下單失敗': 'Order Failed',
    '查詢廣告': 'Search Ads', '我要買': 'I want to buy', '我要賣': 'I want to sell',
    '我要買（收幣）': 'Buy (receive coins)', '我要賣（售幣）': 'Sell (send coins)',
    '銀行轉帳': 'Bank Transfer', '支付寶': 'Alipay', '微信支付': 'WeChat Pay', '街口支付': 'JKOPay', 'LINE Pay': 'LINE Pay',
    '限額': 'Limit', '完成': 'Completed', '評分': 'Rating', '支付': 'Payment', '單價': 'Unit Price',
    '最低金額': 'Min Amount', '最高金額': 'Max Amount', '尚無訂單': 'No orders yet', '尚未發佈廣告': 'No ads yet',
    '待付款': 'Pending Payment', '待放行': 'Pending Release', '已完成': 'Completed', '已取消': 'Cancelled',
    '申訴中': 'Disputed', '申訴': 'Dispute', '確認已付款': 'Confirm Payment', '取消': 'Cancel',
    '放行幣': 'Release Coins', '賣家放行': 'Seller Release', '法幣付款': 'Fiat Payment', '已付款': 'Paid',
    'C2C 場外交易': 'C2C OTC Trading', '買幣 / 賣幣': 'Buy / Sell', '廣告主擔保交易': 'Advertiser-guaranteed trades',
    '法幣直接買賣加密貨幣': 'Buy & sell crypto with fiat',
    '支持 TWD/CNY/USD/HKD/JPY': 'Support TWD/CNY/USD/HKD/JPY',
    '流動性挖礦': 'Liquidity Mining', '借貸市場': 'Lending Market', 'IDO 發行': 'IDO Launch', '收益金庫': 'Vault',
    '流動性池': 'Liquidity Pools', '儲備': 'Reserve', '總LP': 'Total LP', 'APR': 'APR', '動態': 'Dynamic',
    '添加流動性': 'Add Liquidity', '加入流動性': 'Add Liquidity', '我的頭寸': 'My Positions',
    'LP 份額即池內權益': 'LP share equals pool equity', '身份': 'Identity', '讀取': 'Load', '未連接': 'Not Connected',
    '開頭地址': 'address starting with', '開頭': 'starting with', '地址為身份操作': 'Address as identity',
    '存款': 'Deposit', '借款': 'Borrow', '抵押率': 'Collateral Ratio', '總存款': 'Total Deposits', '總借款': 'Total Borrows',
    '存入': 'Deposit', '取出': 'Withdraw', '借出': 'Borrow', '抵押資產': 'Collateral Asset', '抵押數量': 'Collateral Amount',
    '借款資產': 'Borrow Asset', '借款數量': 'Borrow Amount', '我的存款': 'My Deposits', '我的貸款': 'My Loans',
    '每日結息計入市場': 'Daily interest credited to market', '全額還款釋放抵押': 'Full repayment releases collateral',
    '認購': 'Subscribe', '我的認購': 'My Subscriptions', '金庫頭寸': 'Vault Positions', '策略自動復投': 'Auto-compound strategy',
    '流動性挖礦與借貸': 'Liquidity Mining & Lending', '獎勵': 'Reward', '待領獎勵': 'Pending Reward',
    '投入': 'Deposited', '加入': 'Add', '市場': 'Market', '最新區塊': 'Latest Blocks', '出塊': 'Block',
    '驗證人': 'Validators', '驗證節點': 'Validator Nodes', '共識': 'Consensus', '質押': 'Stake', '鎖倉': 'Lock',
    '社區': 'Community', '貼文': 'Posts', '成員': 'Members', '市集': 'Marketplace', '廣告': 'Ads',
    '讚': 'Like', '留言': 'Comment', '分享': 'Share', '發佈': 'Post', '發布': 'Post', '貼文內容': 'Post content',
    '回覆': 'Reply', '已售出': 'Sold', '上架': 'List', '下架': 'Delist', '暫停': 'Pause', '啟用': 'Enable',
    '售': 'Sell', '買': 'Buy', '賣': 'Sell', '序號': 'Order No', '狀態': 'Status',
    '總額': 'Total', '最新成交': 'Recent Trades', '成交': 'Trades', '深度': 'Depth', '訂單簿': 'Order Book',
    '資金池': 'Pool', '閃兌': 'Swap', '自動交易': 'Auto Trading', '機器人': 'Bot', '交易機器人': 'Trading Bot',
    '我的資產': 'My Assets', '資產餘額': 'Asset Balance', '入金': 'Deposit', '提現': 'Withdraw', '劃轉': 'Transfer',
    '交易所資金池': 'Exchange Pool', '預設同源': 'Default same-origin', '同埠合併模式': 'Same-port merge mode',
    '端口 覆蓋': 'Port override', 'RPC 即 web': 'RPC is web', '仍可用': 'still available', '雙保險': 'double insurance',
    '連結': 'Link', '鏈接': 'Link', '網絡': 'Network', '節點狀態': 'Node Status', '總節點': 'Total Nodes',
    '出塊中': 'Mining Blocks', '同步中': 'Syncing', '停止中': 'Stopped', '未知': 'Unknown',
    '手動': 'Manual', '自動': 'Auto', '開啟': 'On', '關閉': 'Off',
    '請稍候': 'Please wait', '操作失敗': 'Operation failed', '請輸入數量': 'Please enter amount',
    '請填寫完整價格與限額': 'Please fill in price and limits', '已創建訂單': 'Order created',
    '確認已完成法幣付款': 'Confirm fiat payment completed?', '已付款請確認': 'Paid, please confirm',
    '載入失敗': 'Load failed', '暫無數據': 'No data', '目前無有效廣告': 'No active ads',
    '尚未發佈廣告': 'No ads published', '無更多': 'No more', '所有權': 'Ownership', '總供應量': 'Total Supply',
    '已挖出': 'Mined', '年衰減': 'Annual Decay', '發行量': 'Issuance', '上限': 'Cap',
    '跨鏈': 'Cross-chain', '跨鏈橋': 'Bridge', '鎖定': 'Locked', '鑄造': 'Minted', '銷毀': 'Burned',
    'L2': 'L2', 'Rollup': 'Rollup', '欺詐證明': 'Fraud Proof', '驗證': 'Verify', '輕客戶端': 'Light Client',
    '挖礦機': 'Miner', '礦機': 'Miner', '我的挖礦機': 'My Miner', '挖礦收益': 'Mining Rewards',
    '效率圈': 'Efficiency Ring', '全網挖礦速率': 'Network Mining Rate', '每分鐘出塊': 'Blocks per minute',
    '收益': 'Rewards', '歷史收益': 'History', '今日收益': 'Today', '累計收益': 'Total Rewards',
    '個人產出': 'My Output', '全鏈產出': 'Chain Output', '開始': 'Start', '結束': 'End', '暫停': 'Pause'
  };
  function reverseMap(m) {
    var r = {};
    var keys = Object.keys(m);
    for (var i = 0; i < keys.length; i++) { r[m[keys[i]]] = keys[i]; }
    return r;
  }
  var en2zh = reverseMap(zh2en);
  function sortedKeys(m) { return Object.keys(m).sort(function (a, b) { return b.length - a.length; }); }
  var zhKeys = sortedKeys(zh2en);
  var enKeys = sortedKeys(en2zh);
  var KEY = 'tac_lang';
  function cur() {
    try {
      var p = new URLSearchParams(window.location.search);
      var q = p.get('lang');
      if (q === 'en' || q === 'zh') { return q; }
      return localStorage.getItem(KEY) || 'en';
    } catch (e) { return 'en'; }
  }
  function replaceTextNodes(map, keys) {
    try {
      var walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, null);
      var nodes = [];
      while (walker.nextNode()) { nodes.push(walker.currentNode); }
      for (var i = 0; i < nodes.length; i++) {
        var n = nodes[i];
        var t = n.nodeValue;
        if (!t) continue;
        var changed = false;
        for (var j = 0; j < keys.length; j++) {
          if (t.indexOf(keys[j]) !== -1) { t = t.split(keys[j]).join(map[keys[j]]); changed = true; }
        }
        if (changed) { n.nodeValue = t; }
      }
    } catch (e) { /* 忽略 */ }
  }
  function apply() {
    var lang = cur();
    var d = dict[lang] || dict.en;
    var els = document.querySelectorAll('[data-i18n]');
    for (var i = 0; i < els.length; i++) {
      var k = els[i].getAttribute('data-i18n');
      if (d[k]) { els[i].textContent = d[k]; }
    }
    // 全域文字節點替換（英文模式：中文→英文；中文模式：英文→中文）。
    if (lang === 'en') { replaceTextNodes(zh2en, zhKeys); }
    else { replaceTextNodes(en2zh, enKeys); }
    var btns = document.querySelectorAll('[data-i18n-lang]');
    for (var j = 0; j < btns.length; j++) {
      btns[j].textContent = lang === 'en' ? '中文' : 'EN';
      btns[j].setAttribute('aria-label', lang === 'en' ? 'Switch to Chinese' : 'Switch to English');
    }
    document.documentElement.lang = lang === 'en' ? 'en' : 'zh-Hant';
  }
  function toggle() {
    var next = cur() === 'en' ? 'zh' : 'en';
    try { localStorage.setItem(KEY, next); } catch (e) {}
    apply();
    try { window.dispatchEvent(new CustomEvent('tac-lang-changed', { detail: { lang: next } })); } catch (e) {}
  }
  // TAC_T(key)：JS 動態字串翻譯（未命中回傳 key 本身）。
  window.TAC_T = function (key) {
    var lang = cur();
    var d = dict[lang] || dict.en;
    return d[key] || key;
  };
  window.TAC_I18N = { cur: cur, apply: apply, toggle: toggle };
  document.addEventListener('DOMContentLoaded', apply);
  apply();
  // 自動套用：JS 動態渲染（innerHTML/append）後自動翻譯，無需各頁手動呼叫。
  try {
    var mo = new MutationObserver(function () {
      clearTimeout(window.__tac_i18n_t);
      window.__tac_i18n_t = setTimeout(function () {
        var lang = cur();
        if (lang === 'en') { replaceTextNodes(zh2en, zhKeys); }
        else { replaceTextNodes(en2zh, enKeys); }
      }, 120);
    });
    mo.observe(document.body, { childList: true, subtree: true });
  } catch (e) { /* 舊瀏覽器忽略 */ }
})();
