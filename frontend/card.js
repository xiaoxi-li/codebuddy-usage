(function () {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const app = () => (window.go && window.go.main && window.go.main.App) || null;
  const runtime = () => window.runtime || null;

  let currentSnapshot = null;
  let toastTimer = null;

  function toast(message) {
    if (!message) return;
    const el = $('toast');
    el.textContent = message;
    el.classList.remove('hidden');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.add('hidden'), 2600);
  }

  function fmt(n) {
    const v = Number(n) || 0;
    return (Math.round(v * 100) / 100).toString();
  }

  function escapeHTML(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[c]);
  }

  function render(snapshot) {
    if (!snapshot) return;
    currentSnapshot = snapshot;
    console.log('[card] render snapshot keys:', Object.keys(snapshot).join(','));

    // 账号
    const profile = snapshot.profile || {};
    const nickname = (profile.nickname || '').trim();
    $('nickname').textContent = nickname || 'CodeBuddy';
    const avatar = $('avatar');
    const fallback = $('avatarFallback');
    if (profile.avatarUrl) {
      avatar.src = profile.avatarUrl;
      avatar.classList.remove('hidden');
      fallback.classList.add('hidden');
    } else {
      avatar.classList.add('hidden');
      fallback.classList.remove('hidden');
      fallback.textContent = (nickname || 'C').slice(0, 1).toUpperCase();
    }

    // 余量
    const remain = Number(snapshot.remain) || 0;
    const total = Number(snapshot.total) || 0;
    $('remain').textContent = fmt(remain);
    const pct = total > 0 ? (remain / total) * 100 : 0;
    $('balanceTotal').textContent = '/ ' + fmt(total) + ' · ' + pct.toFixed(1) + '%';
    $('progressFill').style.width = Math.max(0, Math.min(100, pct)) + '%';

    // 凭据有效期
    const authNote = snapshot.authNote || '';
    let authUntil = '凭据有效期 —';
    if (snapshot.expiresAt) {
      const d = new Date(Number(snapshot.expiresAt));
      if (!isNaN(d.getTime())) {
        authUntil = '凭据有效至 ' + (d.getMonth() + 1) + '-' + d.getDate();
      }
    }
    if (snapshot.mode === 'none' || authNote) authUntil = authNote || '凭据异常';
    $('authUntil').textContent = authUntil;

    // 警告条
    const alertBar = $('alertBar');
    if (snapshot.mode === 'none' || /expired|失效|过期|not signed/i.test(authNote)) {
      $('alertText').textContent = authNote || '未找到登录凭据';
      alertBar.classList.remove('hidden');
    } else {
      alertBar.classList.add('hidden');
    }

    // 套餐
    const rows = snapshot.visible || [];
    const container = $('planRows');
    if (!rows.length) {
      container.innerHTML = '<div class="empty-row">暂无可用套餐</div>';
    } else {
      container.innerHTML = rows.map((p) => (
        '<div class="plan-row">' +
          '<span class="col-name" title="' + escapeHTML(p.name) + '">' + escapeHTML(p.name || '-') + '</span>' +
          '<span class="col-num">' + fmt(p.remain) + '</span>' +
          '<span class="col-num">' + fmt(p.total) + '</span>' +
          '<span class="col-exp">' + escapeHTML((p.expires || '-').slice(0, 10)) + '</span>' +
        '</div>'
      )).join('');
    }
    $('planCount').textContent = String(snapshot.totalVisible || rows.length);
    const more = $('planMore');
    if ((snapshot.totalVisible || 0) > rows.length) {
      more.textContent = '共 ' + snapshot.totalVisible + ' 个套餐，仅显示前 ' + rows.length + ' 条';
      more.classList.remove('hidden');
    } else {
      more.classList.add('hidden');
    }

    // 签到徽章
    const checkin = snapshot.checkin || {};
    const badge = $('checkinBadge');
    badge.className = 'badge';
    const dot = badge.querySelector('.badge-dot');
    const label = badge.querySelector('span:last-child');
    if (checkin.state === 'claimed') {
      badge.classList.add('badge-done');
      label.textContent = checkin.freshlyClaimed
        ? '已签到 +' + fmt(checkin.credit)
        : '✓ 已签到';
    } else if (checkin.state === 'unclaimed') {
      badge.classList.add('badge-todo');
      label.textContent = '○ 未签到';
    } else if (checkin.state === 'unknown') {
      badge.classList.add('badge-err');
      label.textContent = '签到异常';
    } else {
      label.textContent = '签到状态未知';
    }
    void dot;

    // 喵喵旅行
    const buddy = snapshot.buddy;
    const buddyEl = $('buddyStatus');
    if (!buddy) {
      buddyEl.textContent = '✿ 未启用';
    } else if (buddy.state === 'traveling') {
      const s = Number(buddy.countdownSeconds) || 0;
      const hh = String(Math.floor(s / 3600)).padStart(2, '0');
      const mm = String(Math.floor((s % 3600) / 60)).padStart(2, '0');
      const ss = String(s % 60).padStart(2, '0');
      buddyEl.textContent = '✿ 旅行倒计时 ' + hh + ':' + mm + ':' + ss + (buddy.locationName ? ' · ' + buddy.locationName : '');
    } else if (buddy.dailyLimit) {
      const credit = buddy.claimCredit;
      buddyEl.textContent = '✿ 今日已完成' + (credit > 0 ? ' · +' + fmt(credit) : '');
    } else {
      let parts = [];
      if (buddy.claimCredit != null) {
        parts.push(buddy.claimCredit > 0 ? fmt(buddy.claimCredit) + ' 已领' : '无可领');
      } else {
        parts.push('<span class="link" data-action="claim">领积分</span>');
      }
      if (buddy.departed) {
        parts.push(fmt(buddy.durationHours) + 'h');
      } else {
        parts.push('<span class="link" data-action="depart">去旅行</span>');
      }
      buddyEl.innerHTML = '✿ ' + parts.join(' · ');
    }

    $('updatedAt').textContent = '最近更新 ' + (snapshot.updatedAt || '—');
  }

  async function call(name, ...args) {
    const a = app();
    if (!a || typeof a[name] !== 'function') {
      toast('后端尚未就绪');
      return null;
    }
    try {
      return await a[name](...args);
    } catch (e) {
      toast('操作失败：' + (e && e.message ? e.message : e));
      return null;
    }
  }

  async function refresh() {
    toast('正在刷新…');
    await call('Refresh');
  }

  async function claim() {
    const r = await call('Claim');
    if (r == null) return;
    if (r.error) toast('领取失败：' + r.error);
    else if (r.credit > 0) toast('已领取 ' + fmt(r.credit) + ' 积分');
    else toast('暂无可领取的旅行积分');
  }

  async function depart() {
    const r = await call('Depart');
    if (r == null) return;
    if (r.error) toast('派出失败：' + r.error);
    else toast('喵喵已出发，约 ' + fmt(r.hours) + ' 小时');
  }

  function showSettings() {
    $('cardView').classList.add('hidden');
    $('settingsView').classList.remove('hidden');
    loadSettings();
  }

  function closeSettings() {
    $('settingsView').classList.add('hidden');
    $('cardView').classList.remove('hidden');
    if (currentSnapshot) render(currentSnapshot);
  }

  async function loadSettings() {
    let s = await call('GetSettings');
    if (!s) return;
    // GetSettings 返回的是 JSON 字符串，需解析后再读取字段
    if (typeof s === 'string') {
      try { s = JSON.parse(s); } catch (e) { console.error('[card] bad settings payload', e); return; }
    }
    $('setToken').value = s.accessToken || '';
    $('setInterval').value = s.refreshIntervalMinutes == null ? 30 : s.refreshIntervalMinutes;
    $('setApiBase').value = s.apiBase || 'https://www.workbuddy.cn';
    $('setCheckin').checked = !!s.autoCheckin;
    $('setBuddy').checked = !!s.buddyTravel;
    $('setAutostart').checked = !!s.autostart;
  }

  async function saveSettings() {
    const settings = {
      accessToken: $('setToken').value.trim(),
      refreshIntervalMinutes: Math.max(0, parseInt($('setInterval').value, 10) || 0),
      apiBase: $('setApiBase').value,
      autoCheckin: $('setCheckin').checked,
      buddyTravel: $('setBuddy').checked,
      autostart: $('setAutostart').checked
    };
    const ok = await call('SaveSettings', JSON.stringify(settings));
    if (ok !== false) {
      toast('设置已保存，正在刷新…');
      closeSettings();
      await call('Refresh');
    }
  }

  function handleAction(action) {
    switch (action) {
      case 'refresh': return refresh();
      case 'claim': return claim();
      case 'depart': return depart();
      case 'settings': return showSettings();
      case 'closeSettings': return closeSettings();
      case 'saveSettings': return saveSettings();
      case 'fixAuth': return call('FixAuth');
      case 'openUsagePage': return call('OpenUsagePage');
      case 'minimize': return call('Minimize');
      case 'closeWindow': return call('CloseWindow');
      default: return undefined;
    }
  }

  document.addEventListener('click', (e) => {
    const el = e.target.closest('[data-action]');
    if (!el) return;
    e.preventDefault();
    handleAction(el.getAttribute('data-action'));
  });

  // 鼠标进入/离开卡片时通知 Go 侧控制粘性隐藏
  // 标题栏按下时启动无边框窗口拖动（排除按钮）
  document.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return;
    if (e.target.closest('button, input, select, .no-drag')) return;
    if (!e.target.closest('.drag-region')) return;
    const a = app();
    if (a && typeof a.MarkMoved === 'function') a.MarkMoved();
  });

  document.addEventListener('mouseenter', () => {
    const a = app();
    if (a && typeof a.SetPointerInside === 'function') a.SetPointerInside(true);
  }, true);
  document.addEventListener('mouseleave', () => {
    const a = app();
    if (a && typeof a.SetPointerInside === 'function') a.SetPointerInside(false);
  }, true);
  document.addEventListener('mousemove', () => {
    const a = app();
    if (a && typeof a.KeepVisible === 'function') a.KeepVisible();
  });

  function init() {
    const r = runtime();
    if (r && typeof r.EventsOn === 'function') {
      r.EventsOn('usage-update', (payload) => {
        try {
          render(typeof payload === 'string' ? JSON.parse(payload) : payload);
        } catch (err) {
          console.error('[card] bad payload', err);
        }
      });
      r.EventsOn('show-settings', () => showSettings());
    }
    const a = app();
    if (a && typeof a.GetSnapshot === 'function') {
      a.GetSnapshot().then((raw) => {
        if (!raw) return;
        try { render(typeof raw === 'string' ? JSON.parse(raw) : raw); } catch (e) { console.error(e); }
      }).catch(() => undefined);
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  window.__card = { render, toast, handleAction };
})();
