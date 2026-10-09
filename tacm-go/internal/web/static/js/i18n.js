// M35：TAC 鏈全套中英介面切換（JS 層 i18n 字典，不依賴服務端重渲染）。
(function () {
  'use strict';
  var dict = {
    zh: {
      'nav.home': '首頁', 'nav.wallet': '錢包', 'nav.exchange': '交易所', 'nav.dash': '儀表板',
      'nav.mining': '挖礦', 'nav.community': '社群',
      'pill.standby': '待機', 'pill.mining': '挖礦中', 'pill.login': '登入/註冊',
      'pill.tools': '☰ 工具', 'pill.wp': '白皮書', 'pill.en': 'EN', 'pill.lang': '中',
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
      'pill.tools': '☰ Tools', 'pill.wp': 'Whitepaper', 'pill.en': 'EN', 'pill.lang': '中',
      'mining.start': '⚡ Start Mining', 'mining.stop': 'Stop Mining', 'mining.online': 'Mining Online', 'mining.offline': 'Offline',
      'wallet.title': 'TAC Wallet', 'wallet.ledger': 'Recent Ledger (Audit)', 'wallet.transfer': 'Internal Transfer', 'wallet.confirm': 'Confirm Transfer',
      'dash.title': 'Dashboard', 'chain.mined': 'Total TACm Mined', 'chain.tiusd': 'TiUSD Circulation',
      'chain.pool': 'Reward Pool', 'chain.earned': 'My Earnings', 'chain.rate': 'Block Rate',
      'index.title': 'TAC Autonomous Chain', 'index.overview': 'Chain Overview',
      'auth.register': 'Register', 'auth.login': 'Log In', 'auth.email': 'Email', 'auth.password': 'Password',
      'common.copy': 'Copy', 'common.refresh': 'Refresh', 'common.close': 'Close', 'common.confirm': 'Confirm'
    }
  };
  var KEY = 'tac_lang';
  function cur() {
    try {
      var p = new URLSearchParams(window.location.search);
      var q = p.get('lang');
      if (q === 'en' || q === 'zh') { return q; }
      return localStorage.getItem(KEY) || 'zh';
    } catch (e) { return 'zh'; }
  }
  function apply() {
    var lang = cur();
    var d = dict[lang] || dict.zh;
    var els = document.querySelectorAll('[data-i18n]');
    for (var i = 0; i < els.length; i++) {
      var k = els[i].getAttribute('data-i18n');
      if (d[k]) { els[i].textContent = d[k]; }
    }
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
    // 通知各頁 JS 重新渲染動態文字（如有）。
    try { window.dispatchEvent(new CustomEvent('tac-lang-changed', { detail: { lang: next } })); } catch (e) {}
  }
  window.TAC_I18N = { cur: cur, apply: apply, toggle: toggle };
  document.addEventListener('DOMContentLoaded', apply);
  // DOM 已就緒時（腳本在 body 尾）立即套用。
  apply();
})();
