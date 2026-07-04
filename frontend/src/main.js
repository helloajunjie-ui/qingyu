import './style.css';
import { EventsOn } from '../wailsjs/runtime/runtime.js';
import { InitSelf } from '../wailsjs/go/main/App.js';
import { GetCreatorName, IsFirstRun, SetCreatorName, CheckApiKey, SaveConfig, FetchModels, GetConfig, Chat, GetGreet, Shutdown } from '../wailsjs/go/main/App';

// ============================================
// 青羽 - 前端控制器 v2
// 状态机：setup → apikey → widget ↔ console
// 设计要点：所有面板切换带过渡动画，窗口变形丝滑
// ============================================

// ── DOM 引用 ──
const $ = (s) => document.querySelector(s);

const el = {
  setupPanel:    $('#setup-panel'),
  setupName:     $('#setup-name'),
  setupBtn:      $('#setup-btn'),
  setupError:    $('#setup-error'),

  apikeyPanel:   $('#apikey-panel'),
  apiUrlInput:   $('#api-url-input'),
  apikeyInput:   $('#apikey-input'),
  modelSelect:   $('#model-select'),
  fetchModelsBtn:$('#fetch-models-btn'),
  apikeyBtn:     $('#apikey-btn'),
  apikeyError:   $('#apikey-error'),
  btnSettings:   $('#btn-settings'),

  widget:        $('#widget'),
  widgetGlow:    $('#widget-glow'),
  // ===== 【抽象光球&贴边隐藏迭代】新增内核光点 + 轨道环引用 =====
  widgetCore:    $('#widget-core'),
  widgetOrbit:   $('#widget-orbit'),
  widgetStatus:  $('#widget-status'),
  widgetBubble:  $('#widget-bubble'),
  widgetBubbleText: $('#widget-bubble-text'),

  console:       $('#console'),
  consoleBody:   $('#console-body'),
  consoleDot:    $('#console-dot'),
  consoleStatus: $('#console-status'),
  consoleName:   $('#console-name'),
  chatInput:     $('#chat-input'),
  chatSend:      $('#chat-send'),
  welcomeText:   $('#welcome-text'),

  btnMinimize:   $('#btn-minimize'),
  btnClose:      $('#btn-close'),

  // ===== 【前端情绪可视化迭代】新增 DOM 引用 =====
  consoleMoodName:     $('#console-mood-name'),
  consoleIdleTime:     $('#console-idle-time'),
  consoleAutonomicLabel: $('#console-autonomic-label'),
  consoleAffinityFill: $('#console-affinity-fill'),
};

// ── 常量 ──
const WIDGET_SIZE = 80;
const CONSOLE_W = 400;
const CONSOLE_H = 540;
const MARGIN = 20;
const DOCKED_W = 14;   // ===== 【抽象光球&贴边隐藏迭代】贴边窄条宽度 =====
const DOCKED_H = 80;   // ===== 【抽象光球&贴边隐藏迭代】贴边窄条高度 =====
const IDLE_DOCK_TIMEOUT = 600000; // 600s 闲置自动贴边
const HOVER_RETRACT_DELAY = 3000; // 鼠标离开 3s 后自动缩回

// ── 状态 ──
let creatorName = '';
let isProcessing = false;
let autonomicActive = false;
let autonomicLog = [];
let heartbeatState = { beat: 0, rate: 2000, phase: 'resting', mood: 'calm', autonomic: false };
let lastHeartbeatTime = Date.now();
// ===== 【抽象光球&贴边隐藏迭代】贴边隐藏状态变量 =====
let edgeSnapState = 'normal'; // normal | docked-left | docked-right | fully-hidden
let lastInteractionTime = Date.now();
let isHovering = false;
let idleDockTimer = null;
let hoverRetractTimer = null;

// ── 窗口控制 ──
function morphWindow(w, h) {
  try { window.runtime.WindowSetSize(w, h); } catch (_) {}
}

function snapToBottomRight(w, h) {
  try {
    const sw = window.screen.availWidth;
    const sh = window.screen.availHeight;
    window.runtime.WindowSetPosition(sw - w - MARGIN, sh - h - MARGIN);
  } catch (_) {}
}

function snapToCenter(w, h) {
  try {
    const sw = window.screen.availWidth;
    const sh = window.screen.availHeight;
    window.runtime.WindowSetPosition((sw - w) / 2, (sh - h) / 2);
  } catch (_) {}
}

// ===== 【抽象光球&贴边隐藏迭代】shrinkToWidget — 支持贴边状态 =====
function shrinkToWidget() {
  if (edgeSnapState === 'docked-left' || edgeSnapState === 'docked-right') {
    morphWindow(DOCKED_W, DOCKED_H);
    if (edgeSnapState === 'docked-left') {
      try { window.runtime.WindowSetPosition(0, (window.screen.availHeight - DOCKED_H) / 2); } catch (_) {}
    } else {
      try { window.runtime.WindowSetPosition(window.screen.availWidth - DOCKED_W, (window.screen.availHeight - DOCKED_H) / 2); } catch (_) {}
    }
  } else {
    morphWindow(WIDGET_SIZE, WIDGET_SIZE);
    snapToBottomRight(WIDGET_SIZE, WIDGET_SIZE);
  }
}

function expandToConsole() {
  morphWindow(CONSOLE_W, CONSOLE_H);
  snapToBottomRight(CONSOLE_W, CONSOLE_H);
}

// ── 面板切换（带过渡） ──
function hideAll() {
  [el.setupPanel, el.apikeyPanel, el.widget, el.console].forEach(p => p.classList.add('hidden'));
}

function showSetup() {
  hideAll();
  el.setupPanel.classList.remove('hidden');
  el.setupName.focus();
  morphWindow(380, 280);
  snapToCenter(380, 280);
}

function showApiKey() {
  hideAll();
  el.apikeyPanel.classList.remove('hidden');
  morphWindow(380, 360);
  snapToCenter(380, 360);

  GetConfig().then(cfgStr => {
    try {
      const cfg = JSON.parse(cfgStr);
      if (cfg.api_base_url) el.apiUrlInput.value = cfg.api_base_url;
      if (cfg.api_key) el.apikeyInput.value = cfg.api_key;
      if (cfg.model_name) el.modelSelect.dataset.currentModel = cfg.model_name;
    } catch (_) {}
    // 如果 URL 为空，填入默认值（确保能正确持久化到 config.json）
    if (!el.apiUrlInput.value) {
      el.apiUrlInput.value = 'https://api.deepseek.com/v1/chat/completions';
    }
    // 等配置加载完成后再获取模型列表
    loadModels();
  });

  el.apiUrlInput.focus();
}

// ===== 【抽象光球&贴边隐藏迭代】showWidget — 重置贴边状态 =====
function showWidget() {
  hideAll();
  // 如果处于完全收起状态，恢复为 normal
  if (edgeSnapState === 'fully-hidden') {
    edgeSnapState = 'normal';
    el.widget.classList.remove('fully-hidden');
  }
  el.widget.classList.remove('hidden');
  requestAnimationFrame(() => {
    el.widgetGlow.classList.add('active');
  });
  shrinkToWidget();
}

function showConsole() {
  if (isProcessing) return;
  hideAll();
  el.console.classList.remove('hidden');
  el.chatInput.focus();
  scrollToBottom();
  expandToConsole();
  getAndShowGreeting();
}

function hideConsole() {
  showWidget();
  el.chatInput.blur();
}

// ── 问候语 ──
async function getAndShowGreeting() {
  try {
    const greet = await GetGreet();
    if (greet) {
      const wt = document.getElementById('welcome-text');
      if (wt) wt.textContent = greet;
    }
  } catch (_) {}
}

// ── 初始化 ──
async function init() {
  const hasKey = await CheckApiKey();

  if (!hasKey) {
    const firstRun = await IsFirstRun();
    if (firstRun) { showSetup(); }
    else { creatorName = await GetCreatorName(); showApiKey(); }
    return;
  }

  const firstRun = await IsFirstRun();
  if (firstRun) { showSetup(); return; }

  creatorName = await GetCreatorName();
  showWidget();
  setWelcomeMessage();

  // 心跳
  EventsOn('heartbeat', (payload) => {
    try {
      const data = JSON.parse(payload);
      heartbeatState = data;
      lastHeartbeatTime = Date.now();
      updateHeartbeatUI(data);
    } catch (_) {}
  });

  // ===== 【前端情绪可视化迭代】离线检测 — 清除情绪辉光 =====
  setInterval(() => {
    if (Date.now() - lastHeartbeatTime > 3000) {
      el.widgetStatus.className = 'widget-status offline';
      el.widgetGlow.classList.remove('active');
      Object.values(moodGlowMap).forEach(c => el.widgetGlow.classList.remove(c));
      el.widgetGlow.classList.remove('idle-decay');
    }
  }, 1000);

  // 自律事件 — 可折叠展示，默认收起
  EventsOn('autonomic', (payload) => {
    try {
      const data = JSON.parse(payload);
      autonomicActive = true;
      el.widget.classList.add('thinking');

      autonomicLog.push({
        time: data.timestamp,
        thought: data.thought,
        toolResult: data.toolResult
      });
      if (autonomicLog.length > 20) autonomicLog.shift();

      if (!el.console.classList.contains('hidden')) {
        addAutonomicMessage(data);
      }

      setTimeout(() => el.widget.classList.remove('thinking'), 5000);
    } catch (_) {}
  });

  // ===== 【前端情绪可视化迭代】主动聊天事件 — 青羽主动找伙伴聊天 =====
  EventsOn('proactive_chat', (payload) => {
    try {
      const data = JSON.parse(payload);
      if (data.message) {
        const bubble = el.widgetBubble;
        const bubbleText = el.widgetBubbleText;
        bubbleText.textContent = data.message;
        bubble.classList.add('show');

        // 如果控制台已打开，也显示在聊天区（带 source 标签）
        if (!el.console.classList.contains('hidden')) {
          addProactiveChatMessage(data);
        }

        // 8 秒后气泡自动消失
        clearTimeout(window._bubbleTimer);
        window._bubbleTimer = setTimeout(() => {
          bubble.classList.remove('show');
        }, 8000);
      }
    } catch (_) {}
  });

  // 任务进度事件 — 实时显示多步骤任务执行状态
  EventsOn('task_progress', (payload) => {
    try {
      const data = JSON.parse(payload);
      // 找到或更新已有的任务进度消息
      let existing = document.querySelector('.message.task-progress:last-child');
      if (!existing) {
        addTaskProgress(data);
      } else {
        // 更新已有进度条的步骤状态
        const stepsContainer = existing.querySelector('.task-steps');
        if (stepsContainer && data.steps) {
          stepsContainer.innerHTML = '';
          data.steps.forEach((step, i) => {
            const stepDiv = document.createElement('div');
            stepDiv.className = 'task-step';
            const done = step.done ? '✅' : (step.active ? '⏳' : '⬜');
            stepDiv.textContent = `${done} ${step.label}`;
            if (step.done) stepDiv.classList.add('step-done');
            if (step.active) stepDiv.classList.add('step-active');
            stepsContainer.appendChild(stepDiv);
          });
        }
      }
    } catch (_) {}
  });

  // 点击气泡 → 展开控制台，让用户回复
  el.widgetBubble.addEventListener('click', () => {
    // 先移除气泡
    el.widgetBubble.classList.remove('show');
    clearTimeout(window._bubbleTimer);
    // 展开控制台
    showConsole();
  });

  // 自检事件
  EventsOn('selfcheck', (payload) => {
    try {
      const data = JSON.parse(payload);
      if (data.status === 'warning' || data.status === 'error') {
        el.widgetStatus.className = 'widget-status warning';
        el.widgetStatus.title = '⚠️ 自检异常';
      }
      if (!el.console.classList.contains('hidden')) {
        const icon = data.status === 'ok' ? '✅' : '⚠️';
        const msg = document.createElement('div');
        msg.className = 'message system';
        msg.innerHTML = `<span style="opacity:0.5">🔍 自检报告</span><br>${icon} ${data.summary || '未知'}`;
        el.consoleBody.appendChild(msg);
        scrollToBottom();
      }
    } catch (_) {}
  });

  // ===== 【拓展工具集迭代】系统通知事件 — 接收后端推送的系统通知 =====
  EventsOn('system_notify', (payload) => {
    try {
      const data = JSON.parse(payload);
      // 如果控制台已打开，在聊天区显示通知
      if (!el.console.classList.contains('hidden')) {
        const msg = document.createElement('div');
        msg.className = 'message system';
        msg.innerHTML = `<span style="opacity:0.5">🔔 ${data.title || '系统通知'}</span><br>${data.message || ''}`;
        el.consoleBody.appendChild(msg);
        scrollToBottom();
      }
      // 浏览器原生通知（如果已授权）
      if (Notification && Notification.permission === 'granted') {
        new Notification(data.title || '青羽', { body: data.message || '' });
      }
    } catch (_) {}
  });

  // ===== 【拓展工具集迭代】托盘提示事件 — 接收后端推送的托盘气泡 =====
  EventsOn('tray_tip', (payload) => {
    try {
      const data = JSON.parse(payload);
      // 在 widget 气泡区域显示简短提示
      if (data.message) {
        const bubble = el.widgetBubble;
        const bubbleText = el.widgetBubbleText;
        const iconMap = { info: 'ℹ️', warning: '⚠️', error: '❌' };
        bubbleText.textContent = `${iconMap[data.icon] || 'ℹ️'} ${data.message}`;
        bubble.classList.add('show');

        clearTimeout(window._trayTimer);
        window._trayTimer = setTimeout(() => {
          bubble.classList.remove('show');
        }, 5000);
      }
    } catch (_) {}
  });

  // ===== 【抽象光球&贴边隐藏迭代】边缘吸附逻辑 =====
  // 闲置自动贴边定时器
  idleDockTimer = setInterval(() => {
    const idle = Date.now() - lastInteractionTime;
    if (idle > IDLE_DOCK_TIMEOUT && edgeSnapState === 'normal' && el.widget.classList.contains('hidden') === false) {
      // 自动滑向最近边缘（右下角 → 右边缘）
      edgeSnapState = 'docked-right';
      el.widget.classList.add('docked', 'docked-right');
      shrinkToWidget();
    }
  }, 10000);

  // 鼠标进入贴边条 → 展开
  el.widget.addEventListener('mouseenter', () => {
    if (edgeSnapState === 'docked-left' || edgeSnapState === 'docked-right') {
      isHovering = true;
      clearTimeout(hoverRetractTimer);
      // 展开为完整光球
      const wasDockedLeft = edgeSnapState === 'docked-left';
      edgeSnapState = 'normal';
      el.widget.classList.remove('docked', 'docked-left', 'docked-right');
      morphWindow(WIDGET_SIZE, WIDGET_SIZE);
      // 保持在贴边位置（使用展开前的状态判断）
      if (wasDockedLeft) {
        try { window.runtime.WindowSetPosition(0, (window.screen.availHeight - WIDGET_SIZE) / 2); } catch (_) {}
      } else {
        try { window.runtime.WindowSetPosition(window.screen.availWidth - WIDGET_SIZE - MARGIN, (window.screen.availHeight - WIDGET_SIZE) / 2); } catch (_) {}
      }
    }
  });

  // 鼠标离开 → 3s 后自动缩回
  el.widget.addEventListener('mouseleave', () => {
    if (isHovering) {
      isHovering = false;
      clearTimeout(hoverRetractTimer);
      hoverRetractTimer = setTimeout(() => {
        if (!isHovering && edgeSnapState === 'normal') {
          // 判断离哪边近
          try {
            const rect = window.runtime.WindowGetPos();
            // 简化：默认贴右
            edgeSnapState = 'docked-right';
            el.widget.classList.add('docked', 'docked-right');
            shrinkToWidget();
          } catch (_) {
            edgeSnapState = 'docked-right';
            el.widget.classList.add('docked', 'docked-right');
            shrinkToWidget();
          }
        }
      }, HOVER_RETRACT_DELAY);
    }
  });

  // 右键菜单
  el.widget.addEventListener('contextmenu', (e) => {
    e.preventDefault();
    showContextMenu(e.clientX, e.clientY);
  });

  // 点击其他地方关闭右键菜单
  document.addEventListener('click', () => {
    const menu = document.querySelector('.widget-context-menu');
    if (menu) menu.remove();
  });

  // 更新交互时间（点击 widget 时）
  el.widget.addEventListener('mousedown', () => {
    lastInteractionTime = Date.now();
  });
}

// ===== 【抽象光球&贴边隐藏迭代】右键菜单 =====
function showContextMenu(x, y) {
  // 移除旧菜单
  const old = document.querySelector('.widget-context-menu');
  if (old) old.remove();

  const menu = document.createElement('div');
  menu.className = 'widget-context-menu show';

  const item1 = document.createElement('div');
  item1.className = 'ctx-item';
  item1.textContent = '📌 贴边隐藏';
  item1.addEventListener('click', () => {
    edgeSnapState = 'docked-right';
    el.widget.classList.add('docked', 'docked-right');
    shrinkToWidget();
    menu.remove();
  });

  const divider = document.createElement('div');
  divider.className = 'ctx-divider';

  const item2 = document.createElement('div');
  item2.className = 'ctx-item';
  item2.textContent = '⏹ 完全收起';
  item2.addEventListener('click', () => {
    edgeSnapState = 'fully-hidden';
    el.widget.classList.add('fully-hidden');
    // 窗口缩到最小但保持运行
    morphWindow(1, 1);
    try { window.runtime.WindowSetPosition(0, 0); } catch (_) {}
    menu.remove();
  });

  menu.appendChild(item1);
  menu.appendChild(divider);
  menu.appendChild(item2);
  menu.style.left = x + 'px';
  menu.style.top = y + 'px';
  document.body.appendChild(menu);
}

// ── 设置流程 ──
el.setupBtn.addEventListener('click', async () => {
  const name = el.setupName.value.trim();
  if (!name) {
    el.setupError.textContent = '请输入你的名字';
    el.setupError.classList.remove('hidden');
    return;
  }

  el.setupBtn.disabled = true;
  el.setupBtn.textContent = '锚定中...';

  await SetCreatorName(name);
  creatorName = name;

  el.setupPanel.classList.add('hidden');
  el.setupBtn.disabled = false;
  el.setupBtn.textContent = '锚定';

  const hasKey = await CheckApiKey();
  if (!hasKey) showApiKey();
  else { showWidget(); setWelcomeMessage(); }
});

el.setupName.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') el.setupBtn.click();
});

// ── 模型列表 ──
async function loadModels() {
  el.modelSelect.innerHTML = '<option value="">加载中...</option>';
  el.modelSelect.disabled = true;
  try {
    const result = await FetchModels();
    let models;
    try { models = JSON.parse(result); } catch (_) {
      el.modelSelect.innerHTML = `<option value="">${result}</option>`;
      el.modelSelect.disabled = false;
      return;
    }
    if (Array.isArray(models) && models.length > 0) {
      el.modelSelect.innerHTML = '<option value="">-- 选择模型 --</option>'
        + models.map(m => `<option value="${m.id}">${m.id}</option>`).join('');
      const cur = el.modelSelect.dataset.currentModel;
      if (cur) {
        const opt = el.modelSelect.querySelector(`option[value="${cur}"]`);
        if (opt) opt.selected = true;
      }
    } else {
      el.modelSelect.innerHTML = '<option value="">暂无可用模型</option>';
    }
  } catch (_) {
    el.modelSelect.innerHTML = '<option value="">获取失败</option>';
  }
  el.modelSelect.disabled = false;
}

el.fetchModelsBtn.addEventListener('click', loadModels);

// ── API 配置保存 ──
el.apikeyBtn.addEventListener('click', async () => {
  const apiUrl = el.apiUrlInput.value.trim();
  const key = el.apikeyInput.value.trim();
  // 如果用户没选模型（空字符串），取第一个可用模型
  let model = el.modelSelect.value;
  if (!model) {
    const firstOption = el.modelSelect.querySelector('option[value]:not([value=""])');
    if (firstOption) model = firstOption.value;
  }

  if (!key) {
    el.apikeyError.textContent = '请输入 API Key';
    el.apikeyError.classList.remove('hidden');
    return;
  }

  el.apikeyBtn.disabled = true;
  el.apikeyBtn.textContent = '保存中...';

  const result = await SaveConfig(key, apiUrl, model);

  if (result !== '灵魂注入成功') {
    el.apikeyError.textContent = result;
    el.apikeyError.classList.remove('hidden');
    el.apikeyBtn.disabled = false;
    el.apikeyBtn.textContent = '保存配置';
    return;
  }

  el.apikeyPanel.classList.add('hidden');
  el.apikeyBtn.disabled = false;
  el.apikeyBtn.textContent = '保存配置';

  // 判断是否已初始化（已有 creator.json 表示非首次）
  const hasKey = await CheckApiKey();
  const isFirstRun = await IsFirstRun();

  if (!isFirstRun && hasKey) {
    // 二次修改配置 → 直接回到聊天界面
    showWidget();
    setWelcomeMessage();
    showConsole();
    addMessage('✅ 配置已更新', 'bot');
  } else {
    // 首次设置 → 初始化
    showWidget();
    setWelcomeMessage();
    setTimeout(async () => {
      showConsole();
      addMessage('✨ 青羽正在第一次醒来...', 'bot');
      const initResult = await InitSelf();
      const msgs = el.consoleBody.querySelectorAll('.message.bot');
      if (msgs.length > 0) msgs[msgs.length - 1].remove();
      addMessage(initResult, 'bot');
      // 初始化完成后启动自律循环
      await StartAutonomic();
    }, 500);
  }
});

el.apikeyInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') el.apikeyBtn.click();
});
el.apiUrlInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') el.apikeyBtn.click();
});

// ── Widget 交互 ──
el.widget.addEventListener('click', () => {
  if (isProcessing) return;
  showConsole();
});

// ── Console 交互 ──
el.btnMinimize.addEventListener('click', hideConsole);
el.btnClose.addEventListener('click', async () => {
  // 关闭窗口：UI 立即隐藏，后台等待自律循环完成日记后退出
  showWidget();
  el.chatInput.blur();
  await Shutdown();
});
el.btnSettings.addEventListener('click', () => {
  // 从聊天界面打开设置面板
  // 不经过 hideConsole（避免缩成 widget 再弹窗），直接切面板
  el.console.classList.add('hidden');
  el.apikeyPanel.classList.remove('hidden');
  morphWindow(380, 360);
  snapToCenter(380, 360);
  // 加载配置
  GetConfig().then(cfgStr => {
    try {
      const cfg = JSON.parse(cfgStr);
      if (cfg.api_base_url) el.apiUrlInput.value = cfg.api_base_url;
      if (cfg.api_key) el.apikeyInput.value = cfg.api_key;
      if (cfg.model_name) el.modelSelect.dataset.currentModel = cfg.model_name;
    } catch (_) {}
    if (!el.apiUrlInput.value) {
      el.apiUrlInput.placeholder = 'https://api.deepseek.com/v1/chat/completions';
    }
    loadModels();
  });
  el.apiUrlInput.focus();
});

el.chatInput.addEventListener('input', () => {
  el.chatSend.disabled = !el.chatInput.value.trim();
});

el.chatInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    if (!el.chatSend.disabled) sendMessage();
  }
});

el.chatSend.addEventListener('click', sendMessage);

// ── 消息系统 ──
// ===== 【前端情绪可视化迭代】sendMessage — 传递当前 mood =====
async function sendMessage() {
  const text = el.chatInput.value.trim();
  if (!text || isProcessing) return;

  el.chatInput.value = '';
  el.chatSend.disabled = true;

  addMessage(text, 'user');

  isProcessing = true;
  setThinking(true);
  showTyping();

  try {
    const response = await Chat(text);
    removeTyping();

    if (response) {
      addMessage(response, 'bot', heartbeatState.mood);
    } else {
      addMessage('（青羽沉默了）', 'bot', heartbeatState.mood);
    }
  } catch (err) {
    removeTyping();
    const msg = err.message || '';
    if (msg.includes('脑连接断开') || msg.includes('HTTP')) {
      addMessage('😅 我好像连不上大脑了，检查一下设置里的中转站地址？', 'bot', heartbeatState.mood);
    } else {
      addMessage(`😅 出了点小问题：${msg}`, 'bot', heartbeatState.mood);
    }
  } finally {
    isProcessing = false;
    setThinking(false);
    el.chatInput.focus();
  }
}

// ===== 【抽象光球&贴边隐藏迭代】addMessage — 去拟人化 + 情绪跟随文字色 =====
function addMessage(text, role, mood) {
  const div = document.createElement('div');
  div.className = `message ${role}`;

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  // 去拟人化：纯色几何圆，无文字

  const content = document.createElement('div');
  content.className = 'msg-content';
  // bot 消息跟随当前情绪文字色
  if (role === 'bot' && mood && moodGlowMap[mood]) {
    content.classList.add(moodGlowMap[mood]);
  } else if (role === 'bot' && heartbeatState.mood && moodGlowMap[heartbeatState.mood]) {
    content.classList.add(moodGlowMap[heartbeatState.mood]);
  }
  content.innerHTML = formatMessage(text);

  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

function formatMessage(text) {
  const codeBlocks = [];
  let processed = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => {
    const idx = codeBlocks.length;
    codeBlocks.push(`<pre><code>${code.trim()}</code></pre>`);
    return `%%CODEBLOCK_${idx}%%`;
  });

  processed = processed
    .replace(/&/g, '&')
    .replace(/</g, '<')
    .replace(/>/g, '>');

  processed = processed.replace(/`([^`]+)`/g, '<code>$1</code>');
  processed = processed.replace(/\n/g, '<br/>');

  processed = processed.replace(/%%CODEBLOCK_(\d+)%%/g, (_, idx) => {
    return codeBlocks[parseInt(idx)] || '';
  });

  return processed;
}

// ===== 【抽象光球&贴边隐藏迭代】showTyping — 去拟人化 =====
function showTyping() {
  const div = document.createElement('div');
  div.className = 'message bot';
  div.id = 'typing-msg';

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  // 去拟人化：纯色几何圆

  const content = document.createElement('div');
  content.className = 'msg-content';
  content.innerHTML = '<div class="typing-indicator"><span></span><span></span><span></span></div>';

  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

function removeTyping() {
  const typing = document.getElementById('typing-msg');
  if (typing) typing.remove();
}

// ===== 【抽象光球&贴边隐藏迭代】setThinking — 适配双层光球结构 =====
function setThinking(active) {
  if (active) {
    el.widgetGlow.classList.add('active');
    el.widgetCore.classList.add('thinking');
    el.widgetStatus.className = 'widget-status thinking';
    el.consoleDot.className = 'console-dot thinking';
  } else {
    el.widgetCore.classList.remove('thinking');
    el.widgetStatus.className = 'widget-status';
    el.consoleDot.className = 'console-dot';
    // 如果当前有情绪辉光，保留 active
    if (heartbeatState.mood && moodGlowMap[heartbeatState.mood]) {
      el.widgetGlow.classList.add('active');
    } else {
      el.widgetGlow.classList.remove('active');
    }
  }
}

function setWelcomeMessage() {
  const name = creatorName || '伙伴';
  el.welcomeText.innerHTML = `你好，${name}。<br/>我是青羽，你的数字伴侣。<br/><br/>点击输入框开始对话。`;
}

function scrollToBottom() {
  requestAnimationFrame(() => {
    el.consoleBody.scrollTop = el.consoleBody.scrollHeight;
  });
}

// ===== 【抽象光球&贴边隐藏迭代】proactive_chat 消息 — 去拟人化 =====
// ===== 【拓展工具集迭代】新增「网络检索/本地整理」source 溯源标签 =====
function addProactiveChatMessage(data) {
  const div = document.createElement('div');
  div.className = 'message proactive_chat';

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  // 去拟人化：纯色几何圆

  const content = document.createElement('div');
  content.className = 'msg-content';

  // source 标签 — 支持「网络检索」「本地整理」「主动」三类来源
  const sourceDiv = document.createElement('div');
  sourceDiv.className = 'proactive-source';
  const source = data.source || '主动';
  let sourceIcon = '💭';
  let sourceClass = 'source-tag-active';
  if (source === '网络检索') {
    sourceIcon = '🌐';
    sourceClass = 'source-tag-web';
  } else if (source === '本地整理') {
    sourceIcon = '📁';
    sourceClass = 'source-tag-local';
  }
  sourceDiv.innerHTML = `${sourceIcon} ${source} <span class="${sourceClass}">${source}</span>`;

  const msgText = document.createElement('div');
  msgText.innerHTML = formatMessage(data.message);

  content.appendChild(sourceDiv);
  content.appendChild(msgText);

  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

// ===== 自律消息 — 可折叠，默认只显示摘要 =====
function addAutonomicMessage(data) {
  const div = document.createElement('div');
  div.className = 'message autonomic collapsed';

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';

  const content = document.createElement('div');
  content.className = 'msg-content';

  // 摘要行（始终可见）
  const summary = document.createElement('div');
  summary.className = 'autonomic-summary';

  const moodEmoji = moodEmojiMap[heartbeatState.mood] || '💫';
  // 从思考内容提取第一句作为摘要
  const firstLine = (data.thought || '').split('\n')[0] || '思考中...';
  const truncated = firstLine.length > 40 ? firstLine.slice(0, 40) + '...' : firstLine;

  summary.innerHTML = `<span class="autonomic-toggle">▶</span> ${moodEmoji} <span class="autonomic-time">${data.timestamp}</span> ${truncated}`;

  // 详情区（默认隐藏）
  const detail = document.createElement('div');
  detail.className = 'autonomic-detail hidden';

  const thoughtText = document.createElement('div');
  thoughtText.className = 'autonomic-thought';
  thoughtText.innerHTML = formatMessage(data.thought);
  detail.appendChild(thoughtText);

  if (data.toolResult) {
    const toolDiv = document.createElement('div');
    toolDiv.className = 'autonomic-tool';
    toolDiv.textContent = `🛠 ${data.toolResult}`;
    detail.appendChild(toolDiv);
  }

  // 情绪标签
  if (heartbeatState.mood && moodEmojiMap[heartbeatState.mood]) {
    const moodTag = document.createElement('span');
    moodTag.className = 'diary-mood-tag';
    moodTag.textContent = `${moodEmojiMap[heartbeatState.mood]} ${moodNameMap[heartbeatState.mood] || heartbeatState.mood}`;
    summary.appendChild(moodTag);
  }

  // 点击切换折叠
  summary.addEventListener('click', () => {
    div.classList.toggle('collapsed');
    const toggle = summary.querySelector('.autonomic-toggle');
    if (div.classList.contains('collapsed')) {
      detail.classList.add('hidden');
      toggle.textContent = '▶';
    } else {
      detail.classList.remove('hidden');
      toggle.textContent = '▼';
    }
  });

  content.appendChild(summary);
  content.appendChild(detail);
  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

// ===== 任务进度消息 =====
function addTaskProgress(data) {
  const div = document.createElement('div');
  div.className = 'message task-progress';

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';

  const content = document.createElement('div');
  content.className = 'msg-content';

  const header = document.createElement('div');
  header.className = 'task-header';
  header.textContent = `📋 ${data.task || '任务'}`;

  const steps = document.createElement('div');
  steps.className = 'task-steps';

  if (data.steps && Array.isArray(data.steps)) {
    data.steps.forEach((step, i) => {
      const stepDiv = document.createElement('div');
      stepDiv.className = 'task-step';
      const done = step.done ? '✅' : (step.active ? '⏳' : '⬜');
      stepDiv.textContent = `${done} ${step.label}`;
      if (step.done) stepDiv.classList.add('step-done');
      if (step.active) stepDiv.classList.add('step-active');
      steps.appendChild(stepDiv);
    });
  }

  content.appendChild(header);
  content.appendChild(steps);
  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

// ===== 【前端情绪可视化迭代】情绪辉光映射表 =====
const moodGlowMap = {
  curious: 'mood-curious',
  focused: 'mood-focused',
  calm:    'mood-calm',
  hollow:  'mood-hollow',
  aloof:   'mood-aloof',
  warm:    'mood-warm',
};

const moodNameMap = {
  curious: '好奇',
  focused: '专注',
  calm:    '平静',
  hollow:  '空洞',
  aloof:   '疏离',
  warm:    '温暖',
};

const moodEmojiMap = {
  curious: '🔍',
  focused: '🎯',
  calm:    '☁️',
  hollow:  '🌫️',
  aloof:   '❄️',
  warm:    '🌿',
};

// ── 心跳 UI ──
const phaseLabels = {
  active: '活跃', thinking: '思考中', resting: '休憩', sleeping: '休眠'
};
const phaseClasses = {
  active: 'thinking', thinking: 'thinking', resting: '', sleeping: 'offline'
};
const phaseGlow = {
  active: true, thinking: true, resting: false, sleeping: false
};

// ===== 【抽象光球&贴边隐藏迭代】updateHeartbeatUI — 适配双层光球结构 =====
function updateHeartbeatUI(state) {
  // widget-status: phase class + mood class
  const cls = phaseClasses[state.phase] || '';
  el.widgetStatus.className = 'widget-status' + (cls ? ' ' + cls : '');
  const statusMoodClass = moodGlowMap[state.mood];
  if (statusMoodClass && !cls) {
    el.widgetStatus.classList.add(statusMoodClass);
  }

  // 辉光控制：phase 决定是否亮，mood 决定颜色和呼吸
  if (phaseGlow[state.phase]) {
    el.widgetGlow.classList.add('active');
  } else {
    el.widgetGlow.classList.remove('active');
  }

  // 清除旧情绪类，设置新情绪类
  Object.values(moodGlowMap).forEach(c => el.widgetGlow.classList.remove(c));
  const glowMoodClass = moodGlowMap[state.mood];
  if (glowMoodClass && phaseGlow[state.phase]) {
    el.widgetGlow.classList.add(glowMoodClass);
  }

  // moodIntensity 控制辉光透明度（0~1 映射到 opacity 0.3~1.0）
  const intensity = typeof state.moodIntensity === 'number' ? state.moodIntensity : 0.6;
  const glowOpacity = 0.3 + intensity * 0.7;
  el.widgetGlow.style.setProperty('--glow-intensity', glowOpacity);

  // ===== 【抽象光球&贴边隐藏迭代】widget-core 内核光点情绪色同步 =====
  const moodColors = {
    curious: 'rgba(100, 200, 255, 0.9)',
    focused: 'rgba(60, 80, 220, 0.9)',
    calm:    'rgba(140, 180, 170, 0.8)',
    hollow:  'rgba(160, 130, 200, 0.6)',
    aloof:   'rgba(180, 190, 200, 0.4)',
    warm:    'rgba(100, 220, 180, 0.9)',
  };
  const moodGlowColors = {
    curious: 'rgba(100, 200, 255, 0.4)',
    focused: 'rgba(60, 80, 220, 0.4)',
    calm:    'rgba(140, 180, 170, 0.25)',
    hollow:  'rgba(160, 130, 200, 0.2)',
    aloof:   'rgba(180, 190, 200, 0.08)',
    warm:    'rgba(100, 220, 180, 0.35)',
  };
  if (state.mood && moodColors[state.mood]) {
    el.widgetCore.style.background = moodColors[state.mood];
    el.widgetCore.style.boxShadow = `0 0 12px ${moodGlowColors[state.mood]}, 0 0 24px ${moodGlowColors[state.mood]}`;
    el.widgetOrbit.style.borderColor = moodGlowColors[state.mood];
  } else {
    el.widgetCore.style.background = 'var(--accent)';
    el.widgetCore.style.boxShadow = '0 0 12px var(--accent-glow)';
    el.widgetOrbit.style.borderColor = 'rgba(255, 255, 255, 0.06)';
  }

  // idleSeconds > 300 (5min) → 闲置衰减
  const idleSec = typeof state.idleSeconds === 'number' ? state.idleSeconds : 0;
  if (idleSec > 300) {
    el.widgetGlow.classList.add('idle-decay');
    el.widgetCore.style.opacity = 0.4 + (1 - (idleSec - 300) / 900) * 0.6;
  } else {
    el.widgetGlow.classList.remove('idle-decay');
    el.widgetCore.style.opacity = 1;
  }

  // 状态点颜色同步情绪
  if (state.mood && moodGlowMap[state.mood]) {
    el.consoleDot.className = 'console-dot';
    el.consoleDot.classList.add(moodGlowMap[state.mood]);
  }

  // 控制台状态栏更新
  if (!el.console.classList.contains('hidden')) {
    const label = phaseLabels[state.phase] || state.phase;
    const emoji = moodEmojiMap[state.mood] || '💫';
    el.consoleStatus.textContent = `💓 ${state.beat} · ${label} ${emoji}`;

    // 情绪名
    const moodName = moodNameMap[state.mood] || state.mood;
    el.consoleMoodName.textContent = moodName;
    el.consoleMoodName.className = 'console-mood-name';
    if (moodGlowMap[state.mood]) {
      el.consoleMoodName.classList.add(moodGlowMap[state.mood]);
    }

    // 闲置时间
    if (idleSec > 0) {
      const min = Math.floor(idleSec / 60);
      const sec = idleSec % 60;
      el.consoleIdleTime.textContent = min > 0 ? `${min}分${sec}秒` : `${sec}秒`;
      el.consoleIdleTime.classList.remove('hidden');
    } else {
      el.consoleIdleTime.textContent = '';
      el.consoleIdleTime.classList.add('hidden');
    }

    // 自律思考中标签
    if (state.autonomic || autonomicActive) {
      el.consoleAutonomicLabel.classList.remove('hidden');
    } else {
      el.consoleAutonomicLabel.classList.add('hidden');
    }
  }
}

// ── 启动 ──
document.addEventListener('DOMContentLoaded', init);
